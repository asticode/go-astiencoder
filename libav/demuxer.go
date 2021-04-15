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
	ctxFormat             *avformat.Context
	d                     *pktDispatcher
	eh                    *astiencoder.EventHandler
	emulateRate           bool
	interruptRet          *int
	loop                  bool
	p                     *pktPool
	readFrameErrorHandler DemuxerReadFrameErrorHandler
	restamper             PktRestamper
	ss                    map[int]*demuxerStream
	statIncomingRate      *astikit.CounterRateStat
}

type DemuxerReadFrameErrorHandler func(d *Demuxer, ret int) (stop, handled bool)

type demuxerStream struct {
	ctx               Context
	emulateRateNextAt time.Time
	s                 *avformat.Stream
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
	// Custom read frame error handler
	// If handled is false, default error handling will be executed
	ReadFrameErrorHandler DemuxerReadFrameErrorHandler
	// URL of the input
	URL string
}

// NewDemuxer creates a new demuxer
func NewDemuxer(o DemuxerOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (d *Demuxer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDemuxer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("demuxer_%d", count), fmt.Sprintf("Demuxer #%d", count), fmt.Sprintf("Demuxes %s", o.URL), "demuxer")

	// Create demuxer
	d = &Demuxer{
		eh:                    eh,
		emulateRate:           o.EmulateRate,
		loop:                  o.Loop,
		p:                     newPktPool(c),
		readFrameErrorHandler: o.ReadFrameErrorHandler,
		ss:                    make(map[int]*demuxerStream),
		statIncomingRate:      astikit.NewCounterRateStat(),
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pkt dispatcher
	d.d = newPktDispatcher(d, eh, d.p)

	// Add stats
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
			ctx: NewContextFromStream(s),
			s:   s,
		}
	}
	return
}

func (d *Demuxer) addStats() {
	// Get stats
	ss := d.d.stats()
	ss = append(ss, astikit.StatOptions{
		Handler: d.statIncomingRate,
		Metadata: &astikit.StatMetadata{
			Description: "Number of bits going in per second",
			Label:       "Incoming rate",
			Name:        StatNameIncomingRate,
			Unit:        "bps",
		},
	})

	// Add stats
	d.BaseNode.AddStats(ss...)
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
		if d.loop && ret == avutil.AVERROR_EOF {
			// Seek to start
			if ret = d.ctxFormat.AvSeekFrame(-1, d.ctxFormat.StartTime(), avformat.AVSEEK_FLAG_BACKWARD); ret < 0 {
				emitAvError(d, d.eh, ret, "ctxFormat.AvSeekFrame on %s failed", d.ctxFormat.Filename())
				stop = true
			}
		} else {
			// Custom error handler
			if d.readFrameErrorHandler != nil {
				var handled bool
				if stop, handled = d.readFrameErrorHandler(d, ret); handled {
					return
				}
			}

			// Default error handling
			if ret != avutil.AVERROR_EOF {
				emitAvError(d, d.eh, ret, "ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
			}
			stop = true
		}
		return
	}

	// Increment incoming rate
	d.statIncomingRate.Add(float64(pkt.Size() * 8))

	// Get stream
	s, ok := d.ss[pkt.StreamIndex()]
	if !ok {
		return
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
