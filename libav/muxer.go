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
	}
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context, o astiencoder.StartOptions, t astiencoder.CreateTaskFunc) {
	m.BaseNode.Start(ctx, o, t, nil, func(t *astiworker.Task) {
		// Count
		var count int
		defer func(c *int) {
			astilog.Warnf("astilibav: muxed %d pkts", count)
		}(&count)

		// Loop
		// TODO Need one channel
		for {
			select {
			case pkt := <-m.c:
				// TODO Do stuff with the packet
				_ = pkt
				count++
				// TODO If we add a sleep here while activating StopChildrenWhenDone, it only muxes a couple of packets
				// instead of queueing them properly. The context breaks everything.
			case <-m.Context().Done():
				return
			}
		}
	})
}

// HandlePkt implements the PktHandler interface
func (m *Muxer) HandlePkt(pkt *avcodec.Packet) {
	go func() {
		m.c <- pkt
	}()
}
