package astilibav

import (
	"context"
	"fmt"
	"github.com/asticode/go-astilog"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
)

var countEncoder uint64

// Encoder represents an object capable of encoding frames
type Encoder struct {
	*astiencoder.BaseNode
	c                   *astiencoder.Closer
	hs                  []encoderHandlerData
	m                   *sync.Mutex
	packetsBufferLength int
	q                   *astisync.CtxQueue
}

type encoderHandlerData struct {
	h   PktHandler
	pkt *avcodec.Packet
}

// NewEncoder creates a new encoder
func NewEncoder(c *astiencoder.Closer, packetsBufferLength int) (e *Encoder, err error) {
	// Create encoder
	count := atomic.AddUint64(&countEncoder, uint64(1))
	e = &Encoder{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: "Encodes",
			Label:       fmt.Sprintf("Encoder #%d", count),
			Name:        fmt.Sprintf("encoder_%d", count),
		}),
		c:                   c,
		packetsBufferLength: packetsBufferLength,
		q:                   astisync.NewCtxQueue(),
	}
	return
}

// Connect connects the encoder to a PktHandler
func (e *Encoder) Connect(h PktHandler) {
	// Lock
	e.m.Lock()
	defer e.m.Unlock()

	// Create pkt
	pkt := avcodec.AvPacketAlloc()
	e.c.Add(func() error {
		avcodec.AvPacketFree(pkt)
		return nil
	})

	// Append handler
	e.hs = append(e.hs, encoderHandlerData{
		h:   h,
		pkt: pkt,
	})

	// Connect nodes
	n := h.(astiencoder.Node)
	astiencoder.ConnectNodes(e, n)
}

// Start starts the encoder
func (e *Encoder) Start(ctx context.Context, o astiencoder.WorkflowStartOptions, t astiencoder.CreateTaskFunc) {
	e.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Handle context
		go e.q.HandleCtx(e.Context())

		// Create regulator
		r := astisync.NewRegulator(e.Context(), e.packetsBufferLength)
		defer r.Wait()

		// Start queue
		e.q.Start(func(p interface{}) {
			// Assert payload
			f := p.(*avutil.Frame)

			// TODO Encode
			astilog.Warnf("encoding frame %+v", f)
		})
	})
}

// HandleFrame implements the FrameHandler interface
func (e *Encoder) HandleFrame(f *avutil.Frame) {
	e.q.Send(f, true)
}
