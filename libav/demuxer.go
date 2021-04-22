package astilibav

import (
	"context"
	"fmt"
	"sync"
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
	ctx Context
	e   *demuxerRateEmulator
	s   *avformat.Stream
}

// DemuxerOptions represents demuxer options
type DemuxerOptions struct {
	// String content of the demuxer as you would use in ffmpeg
	Dict *Dict
	// If true, the demuxer will sleep between packets for the exact duration of the packet
	EmulateRate bool
	// Max duration of continuous packets of the same stream the demuxer receives has to be
	// inferior to this value.
	// Defaults to 1s
	EmulateRateBufferDuration time.Duration
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
		// Create stream
		ds := &demuxerStream{
			ctx: NewContextFromStream(s),
			s:   s,
		}

		// Create rate emulator
		ds.e = newDemuxerRateEmulator(o.EmulateRateBufferDuration, d.d, d.eh, d.p, s, ds.ctx)

		// Store
		d.ss[s.Index()] = ds
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
			<-d.Context().Done()
			*d.interruptRet = 1
		}()

		// Emulate rate
		wg := &sync.WaitGroup{}
		if d.emulateRate {
			// Loop through streams
			for _, s := range d.ss {
				// Execute the rest in a goroutine
				wg.Add(1)
				go func(e *demuxerRateEmulator) {
					// Make sure to mark task as done
					defer wg.Done()

					// Make sure to stop rate emulator
					defer e.stop()

					// Start rate emulator
					e.start(d.Context())
				}(s.e)
			}
		}

		// Loop
		for {
			// Read frame
			if stop := d.readFrame(d.Context()); stop {
				break
			}

			// Handle pause
			d.HandlePause()

			// Check context
			if d.Context().Err() != nil {
				break
			}
		}

		// Wait for rate emulators
		wg.Wait()
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
		// Add to rate emulator
		s.e.add(ctx, pkt)
	} else {
		// Dispatch pkt
		d.d.dispatch(pkt, s.s)
	}
	return
}

type demuxerRateEmulator struct {
	bufferDuration time.Duration
	c              *astikit.Chan
	cancel         context.CancelFunc
	ctx            context.Context
	d              *pktDispatcher
	eh             *astiencoder.EventHandler
	m              *sync.Mutex // Locks ps
	nextAt         time.Time
	p              *pktPool
	ps             map[*avcodec.Packet]bool
	s              *avformat.Stream
	sc             Context
}

func newDemuxerRateEmulator(bufferDuration time.Duration, d *pktDispatcher, eh *astiencoder.EventHandler, p *pktPool, s *avformat.Stream, sc Context) (e *demuxerRateEmulator) {
	e = &demuxerRateEmulator{
		bufferDuration: bufferDuration,
		c:              astikit.NewChan(astikit.ChanOptions{}),
		d:              d,
		eh:             eh,
		m:              &sync.Mutex{},
		p:              p,
		ps:             make(map[*avcodec.Packet]bool),
		s:              s,
		sc:             sc,
	}
	if e.bufferDuration <= 0 {
		e.bufferDuration = time.Second
	}
	return
}

func (e *demuxerRateEmulator) start(ctx context.Context) {
	// Create context
	e.ctx, e.cancel = context.WithCancel(ctx)

	// Start chan
	e.c.Start(e.ctx)

	// Unref remaining pkts
	e.m.Lock()
	for pkt := range e.ps {
		e.p.put(pkt)
	}
	e.ps = map[*avcodec.Packet]bool{}
	e.m.Unlock()

	// Reset chan
	e.c.Reset()
}

func (e *demuxerRateEmulator) stop() {
	// Cancel context
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}

	// Make sure to stop chan
	e.c.Stop()
}

func (e *demuxerRateEmulator) add(ctx context.Context, in *avcodec.Packet) {
	// Copy pkt
	pkt := e.p.get()
	if ret := pkt.AvPacketRef(in); ret < 0 {
		emitAvError(e, e.eh, ret, "AvPacketRef failed")
		return
	}

	// Store pkt
	e.m.Lock()
	e.ps[pkt] = true
	e.m.Unlock()

	// Get pkt at
	pktAt := e.nextAt
	if pktAt.IsZero() {
		pktAt = time.Now()
	}

	// Compute next at
	e.nextAt = pktAt.Add(time.Duration(avutil.AvRescaleQ(pktDuration(pkt, e.sc), e.s.TimeBase(), nanosecondRational)))

	// Add to chan
	e.c.Add(func() {
		// Make sure to clean pkt
		defer func() {
			// Put pkt
			e.p.put(pkt)

			// Unstore pkt
			e.m.Lock()
			delete(e.ps, pkt)
			e.m.Unlock()
		}()

		// Sleep
		if delta := time.Until(pktAt); delta > 0 {
			astikit.Sleep(e.ctx, delta)
		}

		// Dispatch
		e.d.dispatch(pkt, e.s)
	})

	// Too many pkts are buffered, demuxer needs to wait
	if delta := time.Until(e.nextAt) - e.bufferDuration; delta > 0 {
		// Sleep
		astikit.Sleep(ctx, delta)
	}
}
