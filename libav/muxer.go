package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

var countMuxer uint64

// Muxer represents an object capable of muxing packets into an output
type Muxer struct {
	*astiencoder.BaseNode
	c         *astiencoder.Closer
	ctxFormat *avformat.Context
	e         astiencoder.EmitEventFunc
	o         *sync.Once
	q         *astisync.CtxQueue
}

// NewMuxer creates a new muxer
func NewMuxer(ctxFormat *avformat.Context, e astiencoder.EmitEventFunc, c *astiencoder.Closer) *Muxer {
	count := atomic.AddUint64(&countMuxer, uint64(1))
	return &Muxer{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Muxes to %s", ctxFormat.Filename()),
			Label:       fmt.Sprintf("Muxer #%d", count),
			Name:        fmt.Sprintf("muxer_%d", count),
		}),
		c:         c,
		ctxFormat: ctxFormat,
		e:         e,
		o:         &sync.Once{},
		q:         astisync.NewCtxQueue(),
	}
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context, o astiencoder.WorkflowStartOptions, t astiencoder.CreateTaskFunc) {
	m.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Handle context
		go m.q.HandleCtx(m.Context())

		// Make sure to write header once
		var ret int
		m.o.Do(func() { ret = m.ctxFormat.AvformatWriteHeader(nil) })
		if ret < 0 {
			emitAvError(m.e, ret, "m.ctxFormat.AvformatWriteHeader on %s failed", m.ctxFormat.Filename())
			return
		}

		// Write trailer once everything is done
		m.c.Add(func() error {
			if ret := m.ctxFormat.AvWriteTrailer(); ret < 0 {
				return errors.Wrapf(newAvError(ret), "m.ctxFormat.AvWriteTrailer on %s failed", m.ctxFormat.Filename())
			}
			return nil
		})

		// Start queue
		m.q.Start(func(p interface{}) {
			// Assert payload
			pkt := p.(*avcodec.Packet)

			// Write frame
			if ret := m.ctxFormat.AvInterleavedWriteFrame((*avformat.Packet)(unsafe.Pointer(pkt))); ret < 0 {
				emitAvError(m.e, ret, "m.ctxFormat.AvInterleavedWriteFrame on %+v failed", pkt)
				return
			}
		})
	})
}

// HandlePkt implements the PktHandler interface
func (m *Muxer) HandlePkt(pkt *avcodec.Packet) {
	m.q.Send(pkt, true)
}
