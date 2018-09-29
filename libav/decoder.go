package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

var countDecoder uint64

// Decoder represents an object capable of decoding packets
type Decoder struct {
	*astiencoder.BaseNode
	ctxCodec            *avcodec.Context
	d                   *frameDispatcher
	e                   astiencoder.EmitEventFunc
	packetsBufferLength int
	q                   *astisync.CtxQueue
}

// NewDecoder creates a new decoder
func NewDecoder(ctxCodec *avcodec.Context, e astiencoder.EmitEventFunc, c *astiencoder.Closer, packetsBufferLength int) *Decoder {
	count := atomic.AddUint64(&countDecoder, uint64(1))
	return &Decoder{
		BaseNode: astiencoder.NewBaseNode(e, astiencoder.NodeMetadata{
			Description: "Decodes",
			Label:       fmt.Sprintf("Decoder #%d", count),
			Name:        fmt.Sprintf("decoder_%d", count),
		}),
		ctxCodec:            ctxCodec,
		d:                   newFrameDispatcher(c, e),
		e:                   e,
		packetsBufferLength: packetsBufferLength,
		q:                   astisync.NewCtxQueue(),
	}
}

// NewDecoderFromCodecParams creates a new decoder from codec params
func NewDecoderFromCodecParams(codecParams *avcodec.CodecParameters, e astiencoder.EmitEventFunc, c *astiencoder.Closer, packetsBufferLength int) (d *Decoder, err error) {
	// Find decoder
	var cdc *avcodec.Codec
	if cdc = avcodec.AvcodecFindDecoder(codecParams.CodecId()); cdc == nil {
		err = fmt.Errorf("astilibav: no decoder found for codec id %+v", codecParams.CodecId())
		return
	}

	// Alloc context
	var ctxCodec *avcodec.Context
	if ctxCodec = cdc.AvcodecAllocContext3(); ctxCodec == nil {
		err = fmt.Errorf("astilibav: no context allocated for codec %+v", cdc)
		return
	}

	// Copy codec parameters
	if ret := avcodec.AvcodecParametersToContext(ctxCodec, codecParams); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: avcodec.AvcodecParametersToContext on ctx %+v and codec params %+v failed", ctxCodec, codecParams)
		return
	}

	// Open codec
	if ret := ctxCodec.AvcodecOpen2(cdc, nil); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: d.ctxCodec.AvcodecOpen2 on ctx %+v and codec %+v failed", ctxCodec, cdc)
		return
	}

	// Make sure the codec is closed
	c.Add(func() error {
		if ret := ctxCodec.AvcodecClose(); ret < 0 {
			emitAvError(e, ret, "d.ctxCodec.AvcodecClose on %+v failed", ctxCodec)
		}
		return nil
	})

	// Create decoder
	d = NewDecoder(ctxCodec, e, c, packetsBufferLength)
	return
}

// Connect connects the decoder to a FrameHandler
func (d *Decoder) Connect(h FrameHandler) {
	// Add handler
	d.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(d, h.(astiencoder.Node))
}

// Start starts the decoder
func (d *Decoder) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go d.q.HandleCtx(d.Context())

		// Create regulator
		r := astisync.NewRegulator(d.Context(), d.packetsBufferLength)
		defer r.Wait()

		// Make sure to stop the queue properly
		defer d.q.Stop()

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
				if ret := avcodec.AvcodecReceiveFrame(d.ctxCodec, d.d.f); ret < 0 {
					if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
						emitAvError(d.e, ret, "avcodec.AvcodecReceiveFrame failed")
					}
					return
				}

				// Dispatch frame
				d.d.dispatch(r)
			}
		})
	})
}

// HandlePkt implements the PktHandler interface
func (d *Decoder) HandlePkt(pkt *avcodec.Packet) {
	d.q.Send(pkt, true)
}
