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
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

var (
	countDemuxer       uint64
	nanosecondRational = avutil.NewRational(1, 1e9)
)

// Demuxer represents an object capable of demuxing packets out of an input
type Demuxer struct {
	*astiencoder.BaseNode
	ctxFormat     *avformat.Context
	d             *pktDispatcher
	eh            *astiencoder.EventHandler
	emulateRate   bool
	interruptRet  *int
	loop          bool
	loopFirstPkt  *demuxerPkt
	restamper     PktRestamper
	seekToLive    bool
	ss            map[int]*demuxerStream
	statWorkRatio *astistat.DurationRatioStat
}

type demuxerStream struct {
	ctx               Context
	emulateRateNextAt time.Time
	s                 *avformat.Stream
	seekToLiveLastPkt *demuxerPkt
}

type demuxerPkt struct {
	dts        int64
	duration   int64
	receivedAt time.Time
	s          *avformat.Stream
}

func newDemuxerPkt(pkt *avcodec.Packet, s *avformat.Stream) *demuxerPkt {
	return &demuxerPkt{
		dts:        pkt.Dts(),
		duration:   pkt.Duration(),
		receivedAt: time.Now(),
		s:          s,
	}
}

// DemuxerOptions represents demuxer options
type DemuxerOptions struct {
	// String content of the demuxer as you would use in ffmpeg
	Dict string
	// If true, the demuxer will sleep between packets for the exact duration of the packet
	EmulateRate bool
	// Context used to cancel finding stream info
	FindStreamInfoCtx context.Context
	// Exact input format
	Format *avformat.InputFormat
	// If true, at the end of the input the demuxer will seek to its beginning and start over
	// In this case the packets are restamped
	Loop bool
	// Basic node options
	Node astiencoder.NodeOptions
	// If true, the demuxer will not dispatch packets until, for at least one stream, 2 consecutive packets are received
	// at an interval >= to the first packet's duration
	SeekToLive bool
	// URL of the input
	URL string
}

// NewDemuxer creates a new demuxer
func NewDemuxer(o DemuxerOptions, eh *astiencoder.EventHandler, c *astiencoder.Closer) (d *Demuxer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDemuxer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("demuxer_%d", count), fmt.Sprintf("Demuxer #%d", count), fmt.Sprintf("Demuxes %s", o.URL))

	// Create demuxer
	d = &Demuxer{
		d:             newPktDispatcher(c),
		eh:            eh,
		emulateRate:   o.EmulateRate,
		loop:          o.Loop,
		seekToLive:    o.SeekToLive,
		ss:            make(map[int]*demuxerStream),
		statWorkRatio: astistat.NewDurationRatioStat(),
	}
	d.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(d), eh)
	d.addStats()

	// If loop is enabled, we need to add a restamper
	if d.loop {
		d.restamper = NewPktRestamperWithPktDuration()
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
	// We need to create an intermediate variable to avoid "cgo argument has Go pointer to Go pointer" errors
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

	// Handle find stream info cancellation
	if o.FindStreamInfoCtx != nil {
		// Create context
		findStreamInfoCtx, findStreamInfoCancel := context.WithCancel(o.FindStreamInfoCtx)

		// Handle interrupt
		*d.interruptRet = 0
		go func() {
			<-findStreamInfoCtx.Done()
			if o.FindStreamInfoCtx.Err() != nil {
				*d.interruptRet = 1
			}
		}()

		// Make sure to cancel context so that go routine is closed
		defer findStreamInfoCancel()
	}

	// Retrieve stream information
	if ret := d.ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: ctxFormat.AvformatFindStreamInfo on %+v failed", o)
		return
	}

	// Check whether find stream info has been cancelled
	if o.FindStreamInfoCtx != nil && o.FindStreamInfoCtx.Err() != nil {
		err = errors.Wrap(o.FindStreamInfoCtx.Err(), "astilibav: finding stream info has been cancelled")
		return
	}

	// Index streams
	for _, s := range d.ctxFormat.Streams() {
		d.ss[s.Index()] = &demuxerStream{
			ctx: NewContextFromStream(s),
			s:   s,
		}
	}
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
				emitAvError(d, d.eh, ret, "ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
			}
			stop = true
		} else if d.loopFirstPkt != nil {
			// Seek to first pkt
			if ret = d.ctxFormat.AvSeekFrame(d.loopFirstPkt.s.Index(), d.loopFirstPkt.dts, avformat.AVSEEK_FLAG_BACKWARD); ret < 0 {
				emitAvError(d, d.eh, ret, "ctxFormat.AvSeekFrame on %s with stream idx %v and ts %v failed", d.ctxFormat.Filename(), d.loopFirstPkt.s.Index(), d.loopFirstPkt.dts)
				stop = true
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

	// Seek to live
	if d.seekToLive {
		// Pkt duration is not always filled therefore we need to rely on <current pkt dts> - <previous pkt dts>
		if s.seekToLiveLastPkt == nil || time.Now().Sub(s.seekToLiveLastPkt.receivedAt) < time.Duration(avutil.AvRescaleQ(pkt.Dts()-s.seekToLiveLastPkt.dts, s.s.TimeBase(), nanosecondRational)) {
			s.seekToLiveLastPkt = newDemuxerPkt(pkt, s.s)
			return
		}
		d.seekToLive = false
	}

	// Restamp
	if d.restamper != nil {
		d.restamper.Restamp(pkt)
	}

	// Update loop first packet
	if d.loop && d.loopFirstPkt == nil {
		d.loopFirstPkt = newDemuxerPkt(pkt, s.s)
	}

	// Emulate rate
	if d.emulateRate {
		// Sleep until next at
		if !s.emulateRateNextAt.IsZero() {
			if delta := s.emulateRateNextAt.Sub(time.Now()); delta > 0 {
				astitime.Sleep(ctx, delta)
			}
		} else {
			s.emulateRateNextAt = time.Now()
		}

		// Compute next at
		s.emulateRateNextAt = s.emulateRateNextAt.Add(time.Duration(avutil.AvRescaleQ(d.emulateRatePktDuration(pkt, s.ctx), s.s.TimeBase(), nanosecondRational)))
	}

	// Dispatch pkt
	d.d.dispatch(pkt, s.s)
	return
}

func (d *Demuxer) emulateRatePktDuration(pkt *avcodec.Packet, ctx Context) int64 {
	switch ctx.CodecType {
	case avutil.AVMEDIA_TYPE_AUDIO:
		// Get skip samples side data
		sd := pkt.AvPacketGetSideData(avcodec.AV_PKT_DATA_SKIP_SAMPLES, nil)
		if sd == nil {
			return pkt.Duration()
		}

		// Substract number of samples
		skipStart, skipEnd := avutil.AV_RL32(sd, 0), avutil.AV_RL32(sd, 4)
		return pkt.Duration() - avutil.AvRescaleQ(int64(float64(skipStart+skipEnd)/float64(ctx.SampleRate)*1e9), nanosecondRational, ctx.TimeBase)
	default:
		return pkt.Duration()
	}
}
