package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
)

var (
	countDemuxer       uint64
	nanosecondRational = avutil.NewRational(1, 1e9)
)

// Demuxer represents an object capable of demuxing packets out of an input
type Demuxer struct {
	*astiencoder.BaseNode
	ctxFormat    *avformat.Context
	d            *pktDispatcher
	eh           *astiencoder.EventHandler
	emulateRate  bool
	interruptRet *int
	loop         bool
	p            *pktPool
	restamper    PktRestamper
	seekToLive   bool
	ss           map[int]*demuxerStream
}

type demuxerStream struct {
	ctx               Context
	emulateRateNextAt time.Time
	s                 *avformat.Stream
	seekToLive        bool
	seekToLiveCount   int
	seekToLiveLastPkt *demuxerPkt
}

type demuxerPkt struct {
	dts        int64
	receivedAt time.Time
}

func newDemuxerPkt(pkt *avcodec.Packet) *demuxerPkt {
	return &demuxerPkt{
		dts:        pkt.Dts(),
		receivedAt: time.Now(),
	}
}

// DemuxerOptions represents demuxer options
type DemuxerOptions struct {
	// String content of the demuxer as you would use in ffmpeg
	Dict *Dict
	// If true, the demuxer will sleep between packets for the exact duration of the packet
	EmulateRate bool
	// Exact input format
	Format *avformat.InputFormat
	// If true, at the end of the input the demuxer will seek to its beginning and start over
	// In this case the packets are restamped
	Loop bool
	// Basic node options
	Node astiencoder.NodeOptions
	// Context used to cancel probing
	ProbeCtx context.Context
	// If true, the demuxer will not dispatch packets of a stream until the following occurs at least 3 times :
	// 2 consecutive packets are received at an interval >= to the delta of their DTS
	SeekToLive bool
	// URL of the input
	URL string
}

// NewDemuxer creates a new demuxer
func NewDemuxer(o DemuxerOptions, eh *astiencoder.EventHandler, c *astikit.Closer) (d *Demuxer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDemuxer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("demuxer_%d", count), fmt.Sprintf("Demuxer #%d", count), fmt.Sprintf("Demuxes %s", o.URL), "demuxer")

	// Create demuxer
	d = &Demuxer{
		eh:          eh,
		emulateRate: o.EmulateRate,
		loop:        o.Loop,
		p:           newPktPool(c),
		seekToLive:  o.SeekToLive,
		ss:          make(map[int]*demuxerStream),
	}
	d.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(d), eh)
	d.d = newPktDispatcher(d, eh, d.p)
	d.addStats()

	// If loop is enabled, we need to add a restamper
	if d.loop {
		d.restamper = NewPktRestamperWithPktDuration()
	}

	// Dict
	var dict *avutil.Dictionary
	if o.Dict != nil {
		// Parse dict
		if err = o.Dict.Parse(&dict); err != nil {
			err = fmt.Errorf("astilibav: parsing dict failed: %w", err)
			return
		}

		// Make sure the dict is freed
		defer avutil.AvDictFree(&dict)
	}

	// Alloc ctx
	ctxFormat := avformat.AvformatAllocContext()

	// Set interrupt callback
	d.interruptRet = ctxFormat.SetInterruptCallback()

	// Handle probe cancellation
	if o.ProbeCtx != nil {
		// Create context
		probeCtx, probeCancel := context.WithCancel(o.ProbeCtx)

		// Handle interrupt
		*d.interruptRet = 0
		go func() {
			<-probeCtx.Done()
			if o.ProbeCtx.Err() != nil {
				*d.interruptRet = 1
			}
		}()

		// Make sure to cancel context so that go routine is closed
		defer probeCancel()
	}

	// Open input
	if ret := avformat.AvformatOpenInput(&ctxFormat, o.URL, o.Format, &dict); ret < 0 {
		err = fmt.Errorf("astilibav: avformat.AvformatOpenInput on %+v failed: %w", o, NewAvError(ret))
		return
	}

	// Update ctx
	// We need to create an intermediate variable to avoid "cgo argument has Go pointer to Go pointer" errors
	d.ctxFormat = ctxFormat

	// Make sure the input is properly closed
	c.Add(func() error {
		avformat.AvformatCloseInput(d.ctxFormat)
		return nil
	})

	// Check whether probe has been cancelled
	if o.ProbeCtx != nil && o.ProbeCtx.Err() != nil {
		err = fmt.Errorf("astilibav: probing has been cancelled: %w", o.ProbeCtx.Err())
		return
	}

	// Retrieve stream information
	if ret := d.ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
		err = fmt.Errorf("astilibav: ctxFormat.AvformatFindStreamInfo on %+v failed: %w", o, NewAvError(ret))
		return
	}

	// Check whether probe has been cancelled
	if o.ProbeCtx != nil && o.ProbeCtx.Err() != nil {
		err = fmt.Errorf("astilibav: probing has been cancelled: %w", o.ProbeCtx.Err())
		return
	}

	// Index streams
	for _, s := range d.ctxFormat.Streams() {
		d.ss[s.Index()] = &demuxerStream{
			ctx:        NewContextFromStream(s),
			s:          s,
			seekToLive: o.SeekToLive,
		}
	}
	return
}

func (d *Demuxer) addStats() {
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
	d.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Handle interrupt callback
		*d.interruptRet = 0
		go func() {
			<-d.BaseNode.Context().Done()
			*d.interruptRet = 1
		}()

		// Flush
		if d.seekToLive {
			if ret := d.ctxFormat.AvformatFlush(); ret < 0 {
				emitAvError(d, d.eh, ret, "ctxFormat.AvformatFlush on %s failed", d.ctxFormat.Filename())
			}
		}

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
	pkt := d.p.get()
	defer d.p.put(pkt)

	// Read frame
	if ret := d.ctxFormat.AvReadFrame(pkt); ret < 0 {
		if ret != avutil.AVERROR_EOF || !d.loop {
			if ret != avutil.AVERROR_EOF {
				emitAvError(d, d.eh, ret, "ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
			}
			stop = true
		} else {
			// Seek to start
			if ret = d.ctxFormat.AvSeekFrame(-1, d.ctxFormat.StartTime(), avformat.AVSEEK_FLAG_BACKWARD); ret < 0 {
				emitAvError(d, d.eh, ret, "ctxFormat.AvSeekFrame on %s failed", d.ctxFormat.Filename())
				stop = true
			}
		}
		return
	}

	// Get stream
	s, ok := d.ss[pkt.StreamIndex()]
	if !ok {
		return
	}

	// Seek to live
	if s.seekToLive {
		// Pkt duration is not always filled therefore we need to rely on <current pkt dts> - <previous pkt dts>
		if s.seekToLiveLastPkt != nil && s.seekToLiveLastPkt.dts != avutil.AV_NOPTS_VALUE && time.Since(s.seekToLiveLastPkt.receivedAt) >= time.Duration(avutil.AvRescaleQ(pkt.Dts()-s.seekToLiveLastPkt.dts, s.s.TimeBase(), nanosecondRational)) {
			s.seekToLiveCount++
		}

		// Check count
		// 5 is an arbitrary number
		if s.seekToLiveCount < 5 {
			s.seekToLiveLastPkt = newDemuxerPkt(pkt)
			return
		}
		s.seekToLive = false
	}

	// Restamp
	if d.restamper != nil {
		d.restamper.Restamp(pkt)
	}

	// Emulate rate
	if d.emulateRate {
		// Sleep until next at
		if !s.emulateRateNextAt.IsZero() {
			if delta := time.Until(s.emulateRateNextAt); delta > 0 {
				astikit.Sleep(ctx, delta)
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
