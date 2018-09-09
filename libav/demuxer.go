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
	ctxFormat *avformat.Context
	e         astiencoder.EmitEventFunc
	hs        map[int][]PktHandler // Indexed by stream index
	m         *sync.Mutex
}

// PktHandler represents an object capable of handling packets
type PktHandler interface {
	HandlePkt(pkt *avcodec.Packet)
}

// NewDemuxer creates a new demuxer
func NewDemuxer(ctxFormat *avformat.Context, e astiencoder.EmitEventFunc) *Demuxer {
	c := atomic.AddUint64(&countDemuxer, uint64(1))
	return &Demuxer{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Demuxes %s", ctxFormat.Filename()),
			Label:       fmt.Sprintf("Demuxer #%d", c),
			Name:        fmt.Sprintf("demuxer_%d", c),
		}),
		ctxFormat: ctxFormat,
		e:         e,
		hs:        make(map[int][]PktHandler),
		m:         &sync.Mutex{},
	}
}

// OnPkt adds pkt handlers for a specific stream index
func (d *Demuxer) OnPkt(streamIdx int, hs ...PktHandler) {
	d.m.Lock()
	defer d.m.Unlock()
	for _, h := range hs {
		d.hs[streamIdx] = append(d.hs[streamIdx], h)
		n := h.(astiencoder.Node)
		astiencoder.ConnectNodes(d, n)
	}
	return
}

// Start starts the demuxer
func (d *Demuxer) Start(ctx context.Context, o astiencoder.StartOptions, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Count
		var count int
		defer func(c *int) {
			astilog.Warnf("astilibav: demuxed %d pkts", count)
		}(&count)

		// Make sure all packets are sent
		wg := &sync.WaitGroup{}
		defer wg.Wait()

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
			d.handlePkt(pkt, wg)

			// Check context
			if d.Context().Err() != nil {
				return
			}
		}
	})
}

func (d *Demuxer) handlePkt(pkt *avcodec.Packet, wg *sync.WaitGroup) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Retrieve handlers
	hs, ok := d.hs[pkt.StreamIndex()]
	if !ok {
		return
	}

	// Loop through handlers
	for _, h := range hs {
		// TODO Only allow a certain number of go routines to run per child
		// TODO That way, the demuxer doesn't read everything in memory even though the child takes a lot of time to process one pkt
		wg.Add(1)
		go func(h PktHandler) {
			defer wg.Done()
			h.HandlePkt(pkt)
		}(h)
	}
}
