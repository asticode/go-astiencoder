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

// AddStream adds a stream to the format ctx
func (m *Muxer) AddStream() *avformat.Stream {
	return m.ctxFormat.AvformatNewStream(nil)
}

// CloneStream clones a stream and add it to the format ctx
func (m *Muxer) CloneStream(i *avformat.Stream) (o *avformat.Stream, err error) {
	// Add stream
	o = m.AddStream()

	// Copy codec parameters
	if ret := avcodec.AvcodecParametersCopy(o.CodecParameters(), i.CodecParameters()); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: avcodec.AvcodecParametersCopy from %+v to %+v failed", i.CodecParameters(), o.CodecParameters())
		return
	}

	// Reset codec tag as shown in https://github.com/FFmpeg/FFmpeg/blob/n4.0.2/doc/examples/remuxing.c#L122
	o.CodecParameters().SetCodecTag(0)
	return
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

// MuxHandler represents a mux handler
type MuxHandler interface {
	HandlePkt(pkt *avcodec.Packet)
}

// HandlePkt implements the MuxHandler interface
func (m *Muxer) HandlePkt(pkt *avcodec.Packet) {
	m.q.Send(pkt, true)
}
