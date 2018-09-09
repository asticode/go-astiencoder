package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astitools/sync"

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
	ctxFormat *avformat.Context
	q         *astisync.CtxQueue
}

// NewMuxer creates a new muxer
func NewMuxer(ctxFormat *avformat.Context) *Muxer {
	c := atomic.AddUint64(&countMuxer, uint64(1))
	return &Muxer{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Muxes to %s", ctxFormat.Filename()),
			Label:       fmt.Sprintf("Muxer #%d", c),
			Name:        fmt.Sprintf("muxer_%d", c),
		}),
		ctxFormat: ctxFormat,
		q:         astisync.NewCtxQueue(),
	}
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context, o astiencoder.StartOptions, t astiencoder.CreateTaskFunc) {
	m.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Start queue
		var count int
		m.q.Start(m.Context(), func(p interface{}) {
			count++
		})
		astilog.Warnf("astilibav: muxed %d pkts", count)
	})
}

// HandlePkt implements the PktHandler interface
func (m *Muxer) HandlePkt(pkt *avcodec.Packet) {
	m.q.Send(pkt)
}
