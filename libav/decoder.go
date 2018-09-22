package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

var countDecoder uint64

// Decoder represents an object capable of decoding packets
type Decoder struct {
	*astiencoder.BaseNode
	c                   *astiencoder.Closer
	ctxCodec            *avcodec.Context
	e                   astiencoder.EmitEventFunc
	f                   *avutil.Frame
	hs                  []decoderHandlerData
	m                   *sync.Mutex
	packetsBufferLength int
	q                   *astisync.CtxQueue
	s                   *avformat.Stream
}

type decoderHandlerData struct {
	f *avutil.Frame
	h FrameHandler
}

// NewDecoder creates a new decoder
func NewDecoder(s *avformat.Stream, e astiencoder.EmitEventFunc, c *astiencoder.Closer, packetsBufferLength int) (d *Decoder, err error) {
	// Create decoder
	count := atomic.AddUint64(&countDecoder, uint64(1))
	d = &Decoder{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: "Decodes",
			Label:       fmt.Sprintf("Decoder #%d", count),
			Name:        fmt.Sprintf("decoder_%d", count),
		}),
		c:                   c,
		e:                   e,
		f:                   avutil.AvFrameAlloc(),
		m:                   &sync.Mutex{},
		packetsBufferLength: packetsBufferLength,
		q:                   astisync.NewCtxQueue(),
		s:                   s,
	}

	// Make sure the frame is freed
	c.Add(func() error {
		avutil.AvFrameFree(d.f)
		return nil
	})

	// Find decoder
	var cdc *avcodec.Codec
	if cdc = avcodec.AvcodecFindDecoder(s.CodecParameters().CodecId()); c == nil {
		err = fmt.Errorf("astilibav: no decoder found for codec id %+v", s.CodecParameters().CodecId())
		return
	}

	// Alloc context
	if d.ctxCodec = cdc.AvcodecAllocContext3(); d.ctxCodec == nil {
		err = fmt.Errorf("astilibav: no context allocated for codec %+v", c)
		return
	}

	// Copy codec parameters
	if ret := avcodec.AvcodecParametersToContext(d.ctxCodec, s.CodecParameters()); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: avcodec.AvcodecParametersToContext on ctx %+v and codec params %+v failed", d.ctxCodec, s.CodecParameters())
		return
	}

	// Open codec
	if ret := d.ctxCodec.AvcodecOpen2(cdc, nil); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: d.ctxCodec.AvcodecOpen2 on ctx %+v and codec %+v failed", d.ctxCodec, cdc)
		return
	}

	// Make sure the codec is closed
	c.Add(func() error {
		if ret := d.ctxCodec.AvcodecClose(); ret < 0 {
			emitAvError(e, ret, "d.ctxCodec.AvcodecClose on %+v failed", d.ctxCodec)
		}
		return nil
	})
	return
}

// Connect connects the decoder to a FrameHandler
func (d *Decoder) Connect(h FrameHandler) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Create frame
	f := avutil.AvFrameAlloc()
	d.c.Add(func() error {
		avutil.AvFrameFree(f)
		return nil
	})

	// Append handler
	d.hs = append(d.hs, decoderHandlerData{
		f: f,
		h: h,
	})

	// Connect nodes
	n := h.(astiencoder.Node)
	astiencoder.ConnectNodes(d, n)
}

// Start starts the decoder
func (d *Decoder) Start(ctx context.Context, o astiencoder.WorkflowStartOptions, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Handle context
		go d.q.HandleCtx(d.Context())

		// Create regulator
		r := astisync.NewRegulator(d.Context(), d.packetsBufferLength)
		defer r.Wait()

		// Start queue
		d.q.Start(func(p interface{}) {
			// Assert payload
			pkt := p.(*avcodec.Packet)

			// Send pkt to decoder
			if ret := avcodec.AvcodecSendPacket(d.ctxCodec, pkt); ret < 0 {
				emitAvError(d.e, ret, "avcodec.AvcodecSendPacket failed")
				return
			}

			// Loop
			for {
				// Receive frame
				if ret := avcodec.AvcodecReceiveFrame(d.ctxCodec, d.f); ret < 0 {
					if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
						emitAvError(d.e, ret, "avcodec.AvcodecReceiveFrame failed")
					}
					return
				}

				// Handle frame
				d.handleFrame(r)
			}
		})
	})
}

// HandlePkt implements the PktHandler interface
func (d *Decoder) HandlePkt(pkt *avcodec.Packet) {
	d.q.Send(pkt, true)
}

func (d *Decoder) handleFrame(r *astisync.Regulator) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Create new process
	p := r.NewProcess()

	// Add subprocesses
	p.AddSubprocesses(len(d.hs))

	// Loop through handlers
	for _, h := range d.hs {
		// Copy frame
		if ret := avutil.AvFrameRef(h.f, d.f); ret < 0 {
			emitAvError(d.e, ret, "avutil.AvFrameRef of %+v to %+v failed", d.f, h.f)
			p.SubprocessIsDone()
			continue
		}

		// Handle frame
		go func(h decoderHandlerData) {
			defer p.SubprocessIsDone()
			defer avutil.AvFrameUnref(h.f)
			h.h.HandleFrame(h.f)
		}(h)
	}

	// Wait for one of the subprocess to be done
	p.Wait()
}
