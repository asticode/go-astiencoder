package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
)

var countMuxer uint64

// Muxer represents a muxer
type Muxer struct {
	*astiencoder.BaseNode
	c         chan *avcodec.Packet
	ctxFormat *avformat.Context
	w         *astiencoder.Worker
}

// NewMuxer creates a new muxer
func NewMuxer(ctxFormat *avformat.Context) *Muxer {
	atomic.AddUint64(&countMuxer, uint64(1))
	return &Muxer{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Muxes to %s", ctxFormat.Filename()),
			Label:       fmt.Sprintf("Muxer #%d", countDemuxer),
			Name:        fmt.Sprintf("muxer_%d", countDemuxer),
		}),
		c:         make(chan *avcodec.Packet),
		ctxFormat: ctxFormat,
		w:         astiencoder.NewWorker(),
	}
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	m.w.Start(ctx, t, nil, func(t *astiworker.Task) {
		// Count
		var count int
		defer func(c *int) {
			astilog.Warnf("astilibav: muxed %d pkts", count)
		}(&count)

		// Loop
		for {
			select {
			case pkt := <-m.c:
				// TODO Do stuff with the packet
				_ = pkt
				count++
			case <-m.w.Context().Done():
				return
			}
		}
	})
}

// Stop stops the muxer
func (m *Muxer) Stop() {
	m.w.Stop()
}

// HandlePkt implements the PktHandler interface
func (m *Muxer) HandlePkt(pkt *avcodec.Packet) {
	go func() {
		m.c <- pkt
	}()
}
