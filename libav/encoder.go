package astilibav

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
)

var countEncoder uint64

// Encoder represents an object capable of encoding frames
type Encoder struct {
	*astiencoder.BaseNode
	c                  *astikit.Chan
	ctxCodec           *avcodec.Context
	d                  *pktDispatcher
	eh                 *astiencoder.EventHandler
	previousDescriptor Descriptor
	statIncomingRate   *astikit.CounterAvgStat
	statWorkRatio      *astikit.DurationPercentageStat
}

// EncoderOptions represents encoder options
type EncoderOptions struct {
	Ctx  Context
	Node astiencoder.NodeOptions
}

// NewEncoder creates a new encoder
func NewEncoder(o EncoderOptions, eh *astiencoder.EventHandler, c *astikit.Closer) (e *Encoder, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countEncoder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("encoder_%d", count), fmt.Sprintf("Encoder #%d", count), "Encodes")

	// Create encoder
	e = &Encoder{
		c: astikit.NewChan(astikit.ChanOptions{
			AddStrategy: astikit.ChanAddStrategyBlockWhenStarted,
			ProcessAll:  true,
		}),
		d:                newPktDispatcher(c),
		eh:               eh,
		statIncomingRate: astikit.NewCounterAvgStat(),
		statWorkRatio:    astikit.NewDurationPercentageStat(),
	}
	e.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(e), eh)
	e.addStats()

	// Find encoder
	var cdc *avcodec.Codec
	if len(o.Ctx.CodecName) > 0 {
		if cdc = avcodec.AvcodecFindEncoderByName(o.Ctx.CodecName); cdc == nil {
			err = fmt.Errorf("astilibav: no encoder with name %s", o.Ctx.CodecName)
			return
		}
	} else if o.Ctx.CodecID > 0 {
		if cdc = avcodec.AvcodecFindEncoder(o.Ctx.CodecID); cdc == nil {
			err = fmt.Errorf("astilibav: no encoder with id %+v", o.Ctx.CodecID)
			return
		}
	} else {
		err = errors.New("astilibav: neither codec name nor codec id provided")
		return
	}

	// Check whether the context is valid with the codec
	if err = o.Ctx.validWithCodec(cdc); err != nil {
		err = fmt.Errorf("astilibav: checking whether the context is valid with the codec failed: %w", err)
		return
	}

	// Alloc context
	if e.ctxCodec = cdc.AvcodecAllocContext3(); e.ctxCodec == nil {
		err = errors.New("astilibav: no context allocated")
		return
	}

	// Set shared context parameters
	if o.Ctx.GlobalHeader {
		e.ctxCodec.SetFlags(e.ctxCodec.Flags() | avcodec.AV_CODEC_FLAG_GLOBAL_HEADER)
	}
	if o.Ctx.ThreadCount != nil {
		e.ctxCodec.SetThreadCount(*o.Ctx.ThreadCount)
	}

	// Set media type-specific context parameters
	switch o.Ctx.CodecType {
	case avutil.AVMEDIA_TYPE_AUDIO:
		e.ctxCodec.SetBitRate(int64(o.Ctx.BitRate))
		e.ctxCodec.SetChannelLayout(o.Ctx.ChannelLayout)
		e.ctxCodec.SetChannels(o.Ctx.Channels)
		e.ctxCodec.SetSampleFmt(o.Ctx.SampleFmt)
		e.ctxCodec.SetSampleRate(o.Ctx.SampleRate)
	case avutil.AVMEDIA_TYPE_VIDEO:
		e.ctxCodec.SetBitRate(int64(o.Ctx.BitRate))
		e.ctxCodec.SetFramerate(o.Ctx.FrameRate)
		e.ctxCodec.SetGopSize(o.Ctx.GopSize)
		e.ctxCodec.SetHeight(o.Ctx.Height)
		e.ctxCodec.SetPixFmt(o.Ctx.PixelFormat)
		e.ctxCodec.SetSampleAspectRatio(o.Ctx.SampleAspectRatio)
		e.ctxCodec.SetTimeBase(o.Ctx.TimeBase)
		e.ctxCodec.SetWidth(o.Ctx.Width)
	default:
		err = fmt.Errorf("astilibav: encoder doesn't handle %v codec type", o.Ctx.CodecType)
		return
	}

	// Dict
	var dict *avutil.Dictionary
	if len(o.Ctx.Dict) > 0 {
		// Parse dict
		if ret := avutil.AvDictParseString(&dict, o.Ctx.Dict, "=", ",", 0); ret < 0 {
			err = fmt.Errorf("astilibav: avutil.AvDictParseString on %s failed: %w", o.Ctx.Dict, NewAvError(ret))
			return
		}

		// Make sure the dict is freed
		defer avutil.AvDictFree(&dict)
	}

	// Open codec
	if ret := e.ctxCodec.AvcodecOpen2(cdc, &dict); ret < 0 {
		err = fmt.Errorf("astilibav: d.e.ctxCodec.AvcodecOpen2 failed: %w", NewAvError(ret))
		return
	}

	// Make sure the codec is closed
	c.Add(func() error {
		if ret := e.ctxCodec.AvcodecClose(); ret < 0 {
			emitAvError(nil, eh, ret, "d.e.ctxCodec.AvcodecClose failed")
		}
		return nil
	})
	return
}

func (e *Encoder) addStats() {
	// Add incoming rate
	e.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of frames coming in per second",
		Label:       "Incoming rate",
		Unit:        "fps",
	}, e.statIncomingRate)

	// Add work ratio
	e.Stater().AddStat(astikit.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, e.statWorkRatio)

	// Add dispatcher stats
	e.d.addStats(e.Stater())

	// Add chan stats
	e.c.AddStats(e.Stater())
}

// Connect implements the PktHandlerConnector interface
func (e *Encoder) Connect(h PktHandler) {
	// Add handler
	e.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(e, h)
}

// Disconnect implements the PktHandlerConnector interface
func (e *Encoder) Disconnect(h PktHandler) {
	// Delete handler
	e.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(e, h)
}

// Start starts the encoder
func (e *Encoder) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	e.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer e.d.wait()

		// Make sure to flush the encoder
		defer e.flush()

		// Make sure to stop the chan properly
		defer e.c.Stop()

		// Start chan
		e.c.Start(e.Context())
	})
}

func (e *Encoder) flush() {
	e.encode(&FrameHandlerPayload{})
}

// HandleFrame implements the FrameHandler interface
func (e *Encoder) HandleFrame(p *FrameHandlerPayload) {
	e.c.Add(func() {
		// Handle pause
		defer e.HandlePause()

		// Increment incoming rate
		e.statIncomingRate.Add(1)

		// Encode
		e.encode(p)
	})
}

func (e *Encoder) encode(p *FrameHandlerPayload) {
	// Reset frame attributes
	if p.Frame != nil {
		switch e.ctxCodec.CodecType() {
		case avutil.AVMEDIA_TYPE_VIDEO:
			p.Frame.SetKeyFrame(0)
			p.Frame.SetPictType(avutil.AvPictureType(avutil.AV_PICTURE_TYPE_NONE))
		}
	}

	// Send frame to encoder
	e.statWorkRatio.Begin()
	if ret := avcodec.AvcodecSendFrame(e.ctxCodec, p.Frame); ret < 0 {
		e.statWorkRatio.End()
		emitAvError(e, e.eh, ret, "avcodec.AvcodecSendFrame failed")
		return
	}
	e.statWorkRatio.End()

	// Loop
	for {
		// Receive pkt
		if stop := e.receivePkt(p); stop {
			return
		}
	}
}

func (e *Encoder) receivePkt(p *FrameHandlerPayload) (stop bool) {
	// Get pkt from pool
	pkt := e.d.p.get()
	defer e.d.p.put(pkt)

	// Receive pkt
	e.statWorkRatio.Begin()
	if ret := avcodec.AvcodecReceivePacket(e.ctxCodec, pkt); ret < 0 {
		e.statWorkRatio.End()
		if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
			emitAvError(e, e.eh, ret, "avcodec.AvcodecReceivePacket failed")
		}
		stop = true
		return
	}
	e.statWorkRatio.End()

	// Get descriptor
	d := p.Descriptor
	if d == nil && e.previousDescriptor == nil {
		e.eh.Emit(astiencoder.EventError(e, errors.New("astilibav: no valid descriptor")))
		return
	} else if d == nil {
		d = e.previousDescriptor
	} else {
		e.previousDescriptor = d
	}

	// Set pkt duration based on framerate
	if f := e.ctxCodec.Framerate(); f.Num() > 0 {
		pkt.SetDuration(avutil.AvRescaleQ(int64(1e9/f.ToDouble()), nanosecondRational, d.TimeBase()))
	}

	// Rescale timestamps
	pkt.AvPacketRescaleTs(d.TimeBase(), e.ctxCodec.TimeBase())

	// Dispatch pkt
	e.d.dispatch(pkt, newEncoderDescriptor(e.ctxCodec))
	return
}

// AddStream adds a stream based on the codec ctx
func (e *Encoder) AddStream(ctxFormat *avformat.Context) (o *avformat.Stream, err error) {
	// Add stream
	o = AddStream(ctxFormat)

	// Set codec parameters
	if ret := avcodec.AvcodecParametersFromContext(o.CodecParameters(), e.ctxCodec); ret < 0 {
		err = fmt.Errorf("astilibav: avcodec.AvcodecParametersFromContext from %+v to %+v failed: %w", e.ctxCodec, o.CodecParameters(), NewAvError(ret))
		return
	}

	// Set other attributes
	o.SetTimeBase(e.ctxCodec.TimeBase())
	return
}

// FrameSize returns the encoder frame size
func (e *Encoder) FrameSize() int {
	return e.ctxCodec.FrameSize()
}

type encoderDescriptor struct {
	ctxCodec *avcodec.Context
}

func newEncoderDescriptor(ctxCodec *avcodec.Context) *encoderDescriptor {
	return &encoderDescriptor{ctxCodec: ctxCodec}
}

// TimeBase implements the Descriptor interface
func (d *encoderDescriptor) TimeBase() avutil.Rational {
	return d.ctxCodec.TimeBase()
}
