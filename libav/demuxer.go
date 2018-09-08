package astilibav

import (
	"context"
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

// Demuxer represents a demuxer
type Demuxer struct {
	cs        map[int][]chan *avcodec.Packet // Indexed by stream index
	ctxFormat *avformat.Context
	e         astiencoder.EmitEventFunc
	eofFuncs  []DemuxEOFFunc
	m         *sync.Mutex
	pktFuncs  map[int][]DemuxPktFunc // Indexed by stream index
	w         *worker
}

// DemuxPktFunc represents a method that can handle a pkt
type DemuxPktFunc func(pkt *avcodec.Packet)

// DemuxEOFFunc represents a method that can handle the eof
type DemuxEOFFunc func()

// NewDemuxer creates a new demuxer
func NewDemuxer(ctxFormat *avformat.Context, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) *Demuxer {
	return &Demuxer{
		cs:        make(map[int][]chan *avcodec.Packet),
		ctxFormat: ctxFormat,
		e:         e,
		pktFuncs:  make(map[int][]DemuxPktFunc),
		m:         &sync.Mutex{},
		w:         newWorker(t),
	}
}

// OnPkt adds a pkt handler
func (d *Demuxer) OnPkt(streamIdx int, f DemuxPktFunc) {
	d.m.Lock()
	defer d.m.Unlock()
	d.cs[streamIdx] = append(d.cs[streamIdx], make(chan *avcodec.Packet))
	d.pktFuncs[streamIdx] = append(d.pktFuncs[streamIdx], f)
	return
}

// OnEOF adds an eof handler
func (d *Demuxer) OnEOF(f DemuxEOFFunc) {
	d.m.Lock()
	defer d.m.Unlock()
	d.eofFuncs = append(d.eofFuncs, f)
}

// Start starts the demuxer
func (d *Demuxer) Start(ctx context.Context) {
	d.w.start(ctx, d.startHandlerFuncs, func() {
		// Loop
		var pkt = &avcodec.Packet{}
		for {
			// Read frame
			if err := astiencoder.CtxFunc(ctx, func() error {
				if ret := d.ctxFormat.AvReadFrame(pkt); ret < 0 {
					return errors.Wrapf(newAvError(ret), "astilibav: ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
				}
				return nil
			}); err != nil {
				// Assert
				if v, ok := errors.Cause(err).(AvError); ok && int(v) == avutil.AVERROR_EOF {
					d.handleEOF()
					return
				}
				d.e(astiencoder.EventError(err))
				return
			}

			// TODO Copy packet?

			// Handle packet
			d.handlePkt(pkt)

			// Check context
			if d.w.ctx.Err() != nil {
				return
			}
		}
	})
}

// Stop stops the demuxer
func (d *Demuxer) Stop() {
	d.w.stop()
}

func (d *Demuxer) startHandlerFuncs() {
	d.m.Lock()
	defer d.m.Unlock()
	for streamIdx, cs := range d.cs {
		for idx, c := range cs {
			f := d.pktFuncs[streamIdx][idx]
			go func(c chan *avcodec.Packet, f DemuxPktFunc) {
				for {
					select {
					case pkt := <-c:
						f(pkt)
					case <-d.w.ctx.Done():
						return
					}
				}
			}(c, f)
		}
	}
}

func (d *Demuxer) handlePkt(pkt *avcodec.Packet) {
	d.m.Lock()
	defer d.m.Unlock()
	cs, ok := d.cs[pkt.StreamIndex()]
	if !ok {
		return
	}
	for _, c := range cs {
		go func(c chan *avcodec.Packet) {
			c <- pkt
		}(c)
	}
}

func (d *Demuxer) handleEOF() {
	d.m.Lock()
	defer d.m.Unlock()
	for _, f := range d.eofFuncs {
		go f()
	}
}
