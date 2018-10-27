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
	ctxFormat      *avformat.Context
	d              *pktDispatcher
	e              *astiencoder.EventEmitter
	emulateRate    bool
	interruptRet   *int
	restampStartAt *time.Time
	ss             map[int]*demuxerStream
	statWorkRatio  *astistat.DurationRatioStat
}

type demuxerStream struct {
	nextDts       time.Time
	restampOffset int64
	s             *avformat.Stream
	timeBase      avutil.Rational
}

// Timebase implements the Descriptor interface
func (s *demuxerStream) TimeBase() avutil.Rational {
	return s.timeBase
}

// DemuxerOptions represents demuxer options
type DemuxerOptions struct {
	Dict           string
	EmulateRate    bool
	Format         *avformat.InputFormat
	RestampStartAt *time.Time
	Timebase       func(s *avformat.Stream) avutil.Rational
	URL            string
}

// NewDemuxer creates a new demuxer
func NewDemuxer(o DemuxerOptions, e *astiencoder.EventEmitter, c *astiencoder.Closer) (d *Demuxer, err error) {
	// Create demuxer
	count := atomic.AddUint64(&countDemuxer, uint64(1))
	d = &Demuxer{
		BaseNode: astiencoder.NewBaseNode(e, astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Demuxes %s", o.URL),
			Label:       fmt.Sprintf("Demuxer #%d", count),
			Name:        fmt.Sprintf("demuxer_%d", count),
		}),
		d:              newPktDispatcher(c),
		e:              e,
		emulateRate:    o.EmulateRate,
		restampStartAt: o.RestampStartAt,
		ss:             make(map[int]*demuxerStream),
		statWorkRatio:  astistat.NewDurationRatioStat(),
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

	// Retrieve stream information
	if ret := d.ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: ctxFormat.AvformatFindStreamInfo on %+v failed", o)
		return
	}

	// Index streams
	for _, s := range d.ctxFormat.Streams() {
		ds := &demuxerStream{
			s:        s,
			timeBase: s.TimeBase(),
		}
		if o.Timebase != nil {
			ds.timeBase = o.Timebase(s)
		}
		if o.RestampStartAt != nil {
			ds.restampOffset = avutil.AvRescaleQ(int64(time.Now().Sub(*o.RestampStartAt))-avutil.AvRescaleQ(s.CurDts()*1e9, s.TimeBase(), defaultRational), defaultRational, ds.timeBase) / 1e9
		}
		d.ss[s.Index()] = ds
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
	astiencoder.ConnectNodes(d, h.(astiencoder.Node))
}

// ConnectForStream connects the demuxer to a PktHandler for a specific stream
func (d *Demuxer) ConnectForStream(h PktHandler, i *avformat.Stream) {
	// Add handler
	d.d.addHandler(newPktCond(i, h))

	// Connect nodes
	astiencoder.ConnectNodes(d, h.(astiencoder.Node))
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
	pkt := d.d.getPkt()
	defer d.d.putPkt(pkt)

	// Read frame
	d.statWorkRatio.Add(true)
	if ret := d.ctxFormat.AvReadFrame(pkt); ret < 0 {
		d.statWorkRatio.Done(true)
		if ret != avutil.AVERROR_EOF {
			emitAvError(d.e, ret, "ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
		}
		stop = true
		return
	}
	d.statWorkRatio.Done(true)

	// Get stream
	s, ok := d.ss[pkt.StreamIndex()]
	if !ok {
		return
	}

	// Rescale ts
	pkt.AvPacketRescaleTs(s.s.TimeBase(), s.timeBase)

	// Restamp
	if d.restampStartAt != nil {
		delta := pkt.Pts() - pkt.Dts()
		dts := pkt.Dts() + s.restampOffset
		pkt.SetDts(dts)
		pkt.SetPts(dts + delta)
	}

	// Emulate rate
	if d.emulateRate {
		// Sleep until next DTS
		if !s.nextDts.IsZero() {
			if delta := s.nextDts.Sub(time.Now()); delta > 0 {
				astitime.Sleep(ctx, delta)
			}
		} else {
			// In case of restamp we use the first dts since we know when it should be
			if d.restampStartAt != nil {
				s.nextDts = d.restampStartAt.Add(time.Duration(avutil.AvRescaleQ(pkt.Dts()*1e9, s.timeBase, defaultRational)))
			} else {
				s.nextDts = time.Now()
			}
		}

		// Compute next DTS
		s.nextDts = s.nextDts.Add(time.Duration(avutil.AvRescaleQ(pkt.Duration()*1e9, s.timeBase, defaultRational)))
	}

	// Dispatch pkt
	d.d.dispatch(pkt, s)
	return
}
