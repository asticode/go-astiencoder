package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/time"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

var (
	countDemuxer    uint64
	defaultRational = avutil.NewRational(1, 1)
)

// Demuxer represents an object capable of demuxing packets out of an input
type Demuxer struct {
	*astiencoder.BaseNode
	ctxFormat     *avformat.Context
	d             *pktDispatcher
	e             *astiencoder.EventEmitter
	emulateRate   bool
	interruptRet  *int
	loop          bool
	loopFirstPkt  *demuxerPkt
	loopLastPkt   *demuxerPkt
	restamper     Restamper
	ss            map[int]*demuxerStream
	statWorkRatio *astistat.DurationRatioStat
}

type demuxerPkt struct {
	dts      int64
	duration int64
	s        *avformat.Stream
}

type demuxerStream struct {
	nextDts time.Time
	s       *avformat.Stream
}

// DemuxerOptions represents demuxer options
type DemuxerOptions struct {
	Dict        string
	EmulateRate bool
	Format      *avformat.InputFormat
	Loop        bool
	Restamper   Restamper
	URL         string
}

// NewDemuxer creates a new demuxer
func NewDemuxer(o DemuxerOptions, e *astiencoder.EventEmitter, c astiencoder.CloseFuncAdder) (d *Demuxer, err error) {
	// Create demuxer
	count := atomic.AddUint64(&countDemuxer, uint64(1))
	d = &Demuxer{
		BaseNode: astiencoder.NewBaseNode(e, astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Demuxes %s", o.URL),
			Label:       fmt.Sprintf("Demuxer #%d", count),
			Name:        fmt.Sprintf("demuxer_%d", count),
		}),
		d:             newPktDispatcher(c),
		e:             e,
		emulateRate:   o.EmulateRate,
		loop:          o.Loop,
		restamper:     o.Restamper,
		ss:            make(map[int]*demuxerStream),
		statWorkRatio: astistat.NewDurationRatioStat(),
	}

	// Dict
	var dict *avutil.Dictionary
	if len(o.Dict) > 0 {
		// Parse dict
		if ret := avutil.AvDictParseString(&dict, o.Dict, "=", ",", 0); ret < 0 {
			err = errors.Wrapf(NewAvError(ret), "astilibav: avutil.AvDictParseString on %s failed", o.Dict)
			return
		}

		// Make sure the dict is freed
		defer avutil.AvDictFree(&dict)
	}

	// Alloc ctx
	ctxFormat := avformat.AvformatAllocContext()

	// Set interrupt callback
	d.interruptRet = ctxFormat.SetInterruptCallback()

	// Open input
	if ret := avformat.AvformatOpenInput(&ctxFormat, o.URL, o.Format, &dict); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: avformat.AvformatOpenInput on %+v failed", o)
		return
	}
	d.ctxFormat = ctxFormat

	// Make sure the input is properly closed
	c.Add(func() error {
		avformat.AvformatCloseInput(d.ctxFormat)
		return nil
	})

	// First pkt
	if d.restamper != nil {
		d.restamper.FirstPkt()
	}

	// Retrieve stream information
	if ret := d.ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: ctxFormat.AvformatFindStreamInfo on %+v failed", o)
		return
	}

	// Index streams
	for _, s := range d.ctxFormat.Streams() {
		d.ss[s.Index()] = &demuxerStream{s: s}
	}

	// Add stats
	d.addStats()
	return
}

func (d *Demuxer) addStats() {
	// Add work ratio
	d.Stater().AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, d.statWorkRatio)

	// Add dispatcher stats
	d.d.addStats(d.Stater())
}

// CtxFormat returns the format ctx
func (d *Demuxer) CtxFormat() *avformat.Context {
	return d.ctxFormat
}

// Connect implements the PktHandlerConnector interface
func (d *Demuxer) Connect(h PktHandler) {
	// Add handler
	d.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(d, h)
}

// Disconnect implements the PktHandlerConnector interface
func (d *Demuxer) Disconnect(h PktHandler) {
	// Delete handler
	d.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(d, h)
}

// ConnectForStream connects the demuxer to a PktHandler for a specific stream
func (d *Demuxer) ConnectForStream(h PktHandler, i *avformat.Stream) {
	// Add handler
	d.d.addHandler(newPktCond(i, h))

	// Connect nodes
	astiencoder.ConnectNodes(d, h)
}

// DisconnectForStream disconnects the demuxer from a PktHandler for a specific stream
func (d *Demuxer) DisconnectForStream(h PktHandler, i *avformat.Stream) {
	// Delete handler
	d.d.delHandler(newPktCond(i, h))

	// Disconnect nodes
	astiencoder.DisconnectNodes(d, h)
}

// Start starts the demuxer
func (d *Demuxer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer d.d.wait()

		// Handle interrupt callback
		*d.interruptRet = 0
		go func() {
			<-d.BaseNode.Context().Done()
			*d.interruptRet = 1
		}()

		// Loop
		for {
			// Read frame
			if stop := d.readFrame(ctx); stop {
				return
			}

			// Handle pause
			d.HandlePause()

			// Check context
			if d.Context().Err() != nil {
				return
			}
		}
	})
}

func (d *Demuxer) readFrame(ctx context.Context) (stop bool) {
	// Get pkt from pool
	pkt := d.d.p.get()
	defer d.d.p.put(pkt)

	// Read frame
	d.statWorkRatio.Add(true)
	if ret := d.ctxFormat.AvReadFrame(pkt); ret < 0 {
		d.statWorkRatio.Done(true)
		if ret != avutil.AVERROR_EOF || !d.loop {
			if ret != avutil.AVERROR_EOF {
				emitAvError(d.e, ret, "ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
			}
			stop = true
		} else if d.loopFirstPkt != nil {
			// Seek to first pkt
			if ret = d.ctxFormat.AvSeekFrame(d.loopFirstPkt.s.Index(), d.loopFirstPkt.dts, avformat.AVSEEK_FLAG_BACKWARD); ret < 0 {
				emitAvError(d.e, ret, "ctxFormat.AvSeekFrame on %s with stream idx %v and ts %v failed", d.ctxFormat.Filename(), d.loopFirstPkt.s.Index(), d.loopFirstPkt.dts)
				stop = true
			}

			// Update restamper offsets
			if d.restamper != nil {
				d.restamper.UpdateOffsets(avutil.AvRescaleQ((d.loopLastPkt.dts+d.loopLastPkt.duration)*1e9, d.loopLastPkt.s.TimeBase(), defaultRational) - avutil.AvRescaleQ(d.loopFirstPkt.dts*1e9, d.loopFirstPkt.s.TimeBase(), defaultRational))
			}
		}
		return
	}
	d.statWorkRatio.Done(true)

	// Get stream
	s, ok := d.ss[pkt.StreamIndex()]
	if !ok {
		return
	}

	// Update loop packets
	if d.loop {
		// Update first pkt
		if d.loopFirstPkt == nil {
			d.loopFirstPkt = &demuxerPkt{
				dts:      pkt.Dts(),
				duration: pkt.Duration(),
				s:        s.s,
			}
		}

		// Update last pkt
		d.loopLastPkt = &demuxerPkt{
			dts:      pkt.Dts(),
			duration: pkt.Duration(),
			s:        s.s,
		}
	}

	// Restamp
	if d.restamper != nil {
		d.restamper.Restamp(pkt, s.s)
	}

	// Emulate rate
	if d.emulateRate {
		// Sleep until next DTS
		if !s.nextDts.IsZero() {
			if delta := s.nextDts.Sub(time.Now()); delta > 0 {
				astitime.Sleep(ctx, delta)
			}
		} else {
			if d.restamper != nil {
				s.nextDts = d.restamper.Time(pkt.Dts(), s.s)
			} else {
				s.nextDts = time.Now()
			}
		}

		// Compute next DTS
		s.nextDts = s.nextDts.Add(time.Duration(avutil.AvRescaleQ(pkt.Duration()*1e9, s.s.TimeBase(), defaultRational)))
	}

	// Dispatch pkt
	d.d.dispatch(pkt, s.s)
	return
}
