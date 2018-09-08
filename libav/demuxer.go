package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

var countDemuxer uint64

// Demuxer represents a demuxer
type Demuxer struct {
	*astiencoder.BaseNode
	cs        map[int][]chan *avcodec.Packet // Indexed by stream index
	ctxFormat *avformat.Context
	e         astiencoder.EmitEventFunc
	hs        map[int][]PktHandler // Indexed by stream index
	m         *sync.Mutex
	w         *astiencoder.Worker
}

// PktHandler represents an object capable of handling packets
type PktHandler interface {
	HandlePkt(pkt *avcodec.Packet)
}

// NewDemuxer creates a new demuxer
func NewDemuxer(ctxFormat *avformat.Context, e astiencoder.EmitEventFunc) *Demuxer {
	atomic.AddUint64(&countDemuxer, uint64(1))
	return &Demuxer{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Demuxes %s", ctxFormat.Filename()),
			Label:       fmt.Sprintf("Demuxer #%d", countDemuxer),
			Name:        fmt.Sprintf("demuxer_%d", countDemuxer),
		}),
		cs:        make(map[int][]chan *avcodec.Packet),
		ctxFormat: ctxFormat,
		e:         e,
		hs:        make(map[int][]PktHandler),
		m:         &sync.Mutex{},
		w:         astiencoder.NewWorker(),
	}
}

// OnPkt adds pkt handlers for a specific stream index
func (d *Demuxer) OnPkt(streamIdx int, hs ...PktHandler) {
	d.m.Lock()
	defer d.m.Unlock()
	for _, h := range hs {
		d.cs[streamIdx] = append(d.cs[streamIdx], make(chan *avcodec.Packet))
		d.hs[streamIdx] = append(d.hs[streamIdx], h)
		d.AddChildren(h.(astiencoder.Node))
	}
	return
}

// Start starts the demuxer
func (d *Demuxer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	d.w.Start(ctx, t, d.startHandlerFuncs, func(t *astiworker.Task) {
		// Count
		var count int
		defer func(c *int) {
			astilog.Warnf("astilibav: demuxed %d pkts", count)
		}(&count)

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
					return
				}
				d.e(astiencoder.EventError(err))
				return
			}

			// TODO Copy packet?
			count++

			// Handle packet
			d.handlePkt(pkt)

			// Check context
			if d.w.Context().Err() != nil {
				return
			}
		}
	})
}

// Stop stops the demuxer
func (d *Demuxer) Stop() {
	d.w.Stop()
}

func (d *Demuxer) startHandlerFuncs() {
	d.m.Lock()
	defer d.m.Unlock()
	for streamIdx, cs := range d.cs {
		for idx, c := range cs {
			h := d.hs[streamIdx][idx]
			go func(c chan *avcodec.Packet, h PktHandler) {
				for {
					select {
					case pkt := <-c:
						h.HandlePkt(pkt)
					case <-d.w.Context().Done():
						return
					}
				}
			}(c, h)
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
