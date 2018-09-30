package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/pkg/errors"

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
	ctxCodec *avcodec.Context
	d        *pktDispatcher
	e        astiencoder.EmitEventFunc
	q        *astisync.CtxQueue
	r        *astisync.Regulator
}

// NewEncoder creates a new encoder
func NewEncoder(ctxCodec *avcodec.Context, e astiencoder.EmitEventFunc, c *astiencoder.Closer, packetsBufferLength int) *Encoder {
	count := atomic.AddUint64(&countEncoder, uint64(1))
	return &Encoder{
		BaseNode: astiencoder.NewBaseNode(e, astiencoder.NodeMetadata{
			Description: "Encodes",
			Label:       fmt.Sprintf("Encoder #%d", count),
			Name:        fmt.Sprintf("encoder_%d", count),
		}),
		ctxCodec: ctxCodec,
		d:        newPktDispatcher(c),
		e:        e,
		q:        astisync.NewCtxQueue(),
		r:        astisync.NewRegulator(packetsBufferLength),
	}
}

// EncoderOptions represents encoder options
type EncoderOptions struct {
	CodecID     avcodec.CodecId
	CodecName   string
	CodecType   avcodec.MediaType
	FrameRate   avutil.Rational
	Height      int
	PixelFormat avutil.PixelFormat
	TimeBase    avutil.Rational
	Width       int
}

// NewEncoderFromOptions creates a new encoder based on options
func NewEncoderFromOptions(o EncoderOptions, e astiencoder.EmitEventFunc, c *astiencoder.Closer, packetsBufferLength int) (_ *Encoder, err error) {
	// Find encoder
	var cdc *avcodec.Codec
	if len(o.CodecName) > 0 {
		if cdc = avcodec.AvcodecFindEncoderByName(o.CodecName); cdc == nil {
			err = fmt.Errorf("astilibav: no encoder with name %s", o.CodecName)
			return
		}
	} else if o.CodecID > 0 {
		if cdc = avcodec.AvcodecFindEncoder(o.CodecID); cdc == nil {
			err = fmt.Errorf("astilibav: no encoder with id %+v", o.CodecID)
			return
		}
	} else {
		err = errors.New("astilibav: neither codec name nor codec id provided")
		return
	}

	// Alloc context
	var ctxCodec *avcodec.Context
	if ctxCodec = cdc.AvcodecAllocContext3(); ctxCodec == nil {
		err = fmt.Errorf("astilibav: no context allocated for codec %+v", cdc)
		return
	}

	// Set context parameters
	switch o.CodecType {
	case avutil.AVMEDIA_TYPE_VIDEO:
		ctxCodec.SetFramerate(o.FrameRate)
		ctxCodec.SetHeight(o.Height)
		ctxCodec.SetPixFmt(o.PixelFormat)
		ctxCodec.SetTimeBase(o.TimeBase)
		ctxCodec.SetWidth(o.Width)
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

	// Create encoder
	return NewEncoder(ctxCodec, e, c, packetsBufferLength), nil
}

// Connect connects the encoder to a PktHandler
func (e *Encoder) Connect(h PktHandler) {
	// Append handler
	e.d.addHandler(h, nil)

	// Connect nodes
	astiencoder.ConnectNodes(e, h.(astiencoder.Node))
}

// Start starts the encoder
func (e *Encoder) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	e.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go e.q.HandleCtx(e.Context())

		// Set up regulator
		e.r.HandleCtx(e.Context())
		defer e.r.Wait()

		// Make sure to stop the queue properly
		defer e.q.Stop()

		// Start queue
		e.q.Start(func(p interface{}) {
			// Handle pause
			defer e.HandlePause()

			// Assert payload
			f := p.(*avutil.Frame)

			// Send frame to encoder
			if ret := avcodec.AvcodecSendFrame(e.ctxCodec, f); ret < 0 {
				emitAvError(e.e, ret, "avcodec.AvcodecSendFrame failed")
				return
			}

			// Loop
			for {
				// Receive pkt
				if ret := avcodec.AvcodecReceivePacket(e.ctxCodec, e.d.pkt); ret < 0 {
					if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
						emitAvError(e.e, ret, "avcodec.AvcodecReceivePacket failed")
					}
					return
				}

				// Dispatch pkt
				e.d.dispatch(e.r)
			}
		})
	})
}

// HandleFrame implements the FrameHandler interface
func (e *Encoder) HandleFrame(f *avutil.Frame) {
	e.q.Send(f, true)
}
