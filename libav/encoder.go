package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/goav/avformat"

	"github.com/pkg/errors"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
)

var countEncoder uint64

// Encoder represents an object capable of encoding frames
type Encoder struct {
	*astiencoder.BaseNode
	ctxCodec         *avcodec.Context
	d                *pktDispatcher
	e                *astiencoder.EventEmitter
	q                *astisync.CtxQueue
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// NewEncoder creates a new encoder
func NewEncoder(ctxCodec *avcodec.Context, ee *astiencoder.EventEmitter, c *astiencoder.Closer) (e *Encoder) {
	count := atomic.AddUint64(&countEncoder, uint64(1))
	e = &Encoder{
		BaseNode: astiencoder.NewBaseNode(ee, astiencoder.NodeMetadata{
			Description: "Encodes",
			Label:       fmt.Sprintf("Encoder #%d", count),
			Name:        fmt.Sprintf("encoder_%d", count),
		}),
		ctxCodec:         ctxCodec,
		d:                newPktDispatcher(c),
		e:                ee,
		q:                astisync.NewCtxQueue(),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	e.addStats()
	return
}

// NewEncoderFromContext creates a new encoder based on a context
func NewEncoderFromContext(ctx Context, e *astiencoder.EventEmitter, c *astiencoder.Closer) (_ *Encoder, err error) {
	// Find encoder
	var cdc *avcodec.Codec
	if len(ctx.CodecName) > 0 {
		if cdc = avcodec.AvcodecFindEncoderByName(ctx.CodecName); cdc == nil {
			err = fmt.Errorf("astilibav: no encoder with name %s", ctx.CodecName)
			return
		}
	} else if ctx.CodecID > 0 {
		if cdc = avcodec.AvcodecFindEncoder(ctx.CodecID); cdc == nil {
			err = fmt.Errorf("astilibav: no encoder with id %+v", ctx.CodecID)
			return
		}
	} else {
		err = errors.New("astilibav: neither codec name nor codec id provided")
		return
	}

	// Check whether the context is valid with the codec
	if err = ctx.validWithCodec(cdc); err != nil {
		err = errors.Wrap(err, "astilibav: checking whether the context is valid with the codec failed")
		return
	}

	// Alloc context
	var ctxCodec *avcodec.Context
	if ctxCodec = cdc.AvcodecAllocContext3(); ctxCodec == nil {
		err = errors.New("astilibav: no context allocated")
		return
	}

	// Set global context parameters
	ctxCodec.SetFlags(ctxCodec.Flags() | avcodec.AV_CODEC_FLAG_GLOBAL_HEADER)
	if ctx.ThreadCount != nil {
		ctxCodec.SetThreadCount(*ctx.ThreadCount)
	}

	// Set media type-specific context parameters
	switch ctx.CodecType {
	case avutil.AVMEDIA_TYPE_AUDIO:
		ctxCodec.SetBitRate(int64(ctx.BitRate))
		ctxCodec.SetChannelLayout(ctx.ChannelLayout)
		ctxCodec.SetChannels(ctx.Channels)
		ctxCodec.SetSampleFmt(ctx.SampleFmt)
		ctxCodec.SetSampleRate(ctx.SampleRate)
	case avutil.AVMEDIA_TYPE_VIDEO:
		ctxCodec.SetBitRate(int64(ctx.BitRate))
		ctxCodec.SetFramerate(ctx.FrameRate)
		ctxCodec.SetGopSize(ctx.GopSize)
		ctxCodec.SetHeight(ctx.Height)
		ctxCodec.SetPixFmt(ctx.PixelFormat)
		ctxCodec.SetSampleAspectRatio(ctx.SampleAspectRatio)
		ctxCodec.SetTimeBase(ctx.TimeBase)
		ctxCodec.SetWidth(ctx.Width)
	default:
		err = fmt.Errorf("astilibav: encoder doesn't handle %v codec type", ctx.CodecType)
		return
	}

	// Dict
	var dict *avutil.Dictionary
	if len(ctx.Dict) > 0 {
		// Parse dict
		if ret := avutil.AvDictParseString(&dict, ctx.Dict, "=", ",", 0); ret < 0 {
			err = errors.Wrapf(NewAvError(ret), "astilibav: avutil.AvDictParseString on %s failed", ctx.Dict)
			return
		}

		// Make sure the dict is freed
		defer avutil.AvDictFree(&dict)
	}

	// Open codec
	if ret := ctxCodec.AvcodecOpen2(cdc, &dict); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: d.ctxCodec.AvcodecOpen2 failed")
		return
	}

	// Make sure the codec is closed
	c.Add(func() error {
		if ret := ctxCodec.AvcodecClose(); ret < 0 {
			emitAvError(e, ret, "d.ctxCodec.AvcodecClose failed")
		}
		return nil
	})

	// Create encoder
	return NewEncoder(ctxCodec, e, c), nil
}

func (e *Encoder) addStats() {
	// Add incoming rate
	e.Stater().AddStat(astistat.StatMetadata{
		Description: "Number of frames coming in the encoder per second",
		Label:       "Incoming rate",
		Unit:        "fps",
	}, e.statIncomingRate)

	// Add work ratio
	e.Stater().AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, e.statWorkRatio)

	// Add dispatcher stats
	e.d.addStats(e.Stater())

	// Add queue stats
	e.q.AddStats(e.Stater())
}

// Connect connects the encoder to a PktHandler
func (e *Encoder) Connect(h PktHandler) {
	// Append handler
	e.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(e, h.(astiencoder.Node))
}

// Start starts the encoder
func (e *Encoder) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	e.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go e.q.HandleCtx(e.Context())

		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer e.d.wait()

		// We need to create a prev pool since the encoder keeps a buffer of frame
		var pp []Descriptor

		// Make sure to flush the encoder
		defer e.flush(&pp)

		// Make sure to stop the queue properly
		defer e.q.Stop()

		// Start queue
		e.q.Start(func(dp interface{}) {
			// Handle pause
			defer e.HandlePause()

			// Assert payload
			p := dp.(*FrameHandlerPayload)

			// Increment incoming rate
			e.statIncomingRate.Add(1)

			// Encode
			e.encode(p, &pp)
		})
	})
}

func (e *Encoder) flush(pp *[]Descriptor) {
	e.encode(&FrameHandlerPayload{}, pp)
}

func (e *Encoder) encode(p *FrameHandlerPayload, pp *[]Descriptor) {
	// Add prev to pool
	if p.Prev != nil {
		*pp = append(*pp, p.Prev)
	}

	// Reset frame attributes
	if p.Frame != nil {
		p.Frame.Key_frame = 0
		p.Frame.Pict_type = avutil.AV_PICTURE_TYPE_NONE
	}

	// Send frame to encoder
	e.statWorkRatio.Add(true)
	if ret := avcodec.AvcodecSendFrame(e.ctxCodec, p.Frame); ret < 0 {
		e.statWorkRatio.Done(true)
		emitAvError(e.e, ret, "avcodec.AvcodecSendFrame failed")
		return
	}
	e.statWorkRatio.Done(true)

	// Loop
	prev := Descriptor(nil)
	for {
		// Receive pkt
		if stop := e.receivePkt(&prev, pp); stop {
			return
		}
	}
}

func (e *Encoder) receivePkt(prev *Descriptor, pp *[]Descriptor) (stop bool) {
	// Get pkt from pool
	pkt := e.d.getPkt()
	defer e.d.putPkt(pkt)

	// Receive pkt
	e.statWorkRatio.Add(true)
	if ret := avcodec.AvcodecReceivePacket(e.ctxCodec, pkt); ret < 0 {
		e.statWorkRatio.Done(true)
		if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
			emitAvError(e.e, ret, "avcodec.AvcodecReceivePacket failed")
		}
		stop = true
		return
	}
	e.statWorkRatio.Done(true)

	// TODO libx264 returns a pkt with a duration set to 0 here :(

	// Get prev
	if *prev == nil {
		*prev = (*pp)[0]
		*pp = (*pp)[1:]
	}

	// Rescale timestamps
	pkt.AvPacketRescaleTs((*prev).TimeBase(), e.ctxCodec.TimeBase())

	// Dispatch pkt
	e.d.dispatch(pkt, newEncoderPrev(e.ctxCodec))
	return
}

// HandleFrame implements the FrameHandler interface
func (e *Encoder) HandleFrame(p *FrameHandlerPayload) {
	e.q.Send(p)
}

// AddStream adds a stream based on the codec ctx
func (e *Encoder) AddStream(ctxFormat *avformat.Context) (o *avformat.Stream, err error) {
	// Add stream
	o = AddStream(ctxFormat)

	// Set codec parameters
	if ret := avcodec.AvcodecParametersFromContext(o.CodecParameters(), e.ctxCodec); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: avcodec.AvcodecParametersFromContext from %+v to %+v failed", e.ctxCodec, o.CodecParameters())
		return
	}

	// Set other attributes
	o.SetTimeBase(e.ctxCodec.TimeBase())
	return
}

type encoderPrev struct {
	ctxCodec *avcodec.Context
}

func newEncoderPrev(ctxCodec *avcodec.Context) *encoderPrev {
	return &encoderPrev{ctxCodec: ctxCodec}
}

// TimeBase implements the Descriptor interface
func (p *encoderPrev) TimeBase() avutil.Rational {
	return p.ctxCodec.TimeBase()
}
