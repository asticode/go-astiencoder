package astilibav

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countEncoder uint64

// Encoder represents an object capable of encoding frames
type Encoder struct {
	*astiencoder.BaseNode
	c                  *astikit.Chan
	codecCtx           *astiav.CodecContext
	d                  *pktDispatcher
	eh                 *astiencoder.EventHandler
	fp                 *framePool
	pp                 *pktPool
	previousDescriptor Descriptor
	statIncomingRate   *astikit.CounterRateStat
	statProcessedRate  *astikit.CounterRateStat
}

// EncoderOptions represents encoder options
type EncoderOptions struct {
	Ctx  Context
	Node astiencoder.NodeOptions
}

// NewEncoder creates a new encoder
func NewEncoder(o EncoderOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (e *Encoder, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countEncoder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("encoder_%d", count), fmt.Sprintf("Encoder #%d", count), "Encodes", "encoder")

	// Create encoder
	e = &Encoder{
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:                eh,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	e.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, e, astiencoder.EventTypeToNodeEventName)

	// Create pools
	e.fp = newFramePool(e)
	e.pp = newPktPool(e)

	// Create pkt dispatcher
	e.d = newPktDispatcher(e, eh)

	// Add stats
	e.addStats()

	// Find encoder
	var codec *astiav.Codec
	if len(o.Ctx.CodecName) > 0 {
		if codec = astiav.FindEncoderByName(o.Ctx.CodecName); codec == nil {
			err = fmt.Errorf("astilibav: no encoder with name %s", o.Ctx.CodecName)
			return
		}
	} else if o.Ctx.CodecID > 0 {
		if codec = astiav.FindEncoder(o.Ctx.CodecID); codec == nil {
			err = fmt.Errorf("astilibav: no encoder with codec id %s", o.Ctx.CodecID)
			return
		}
	} else {
		err = errors.New("astilibav: neither codec name nor codec id provided")
		return
	}

	// Alloc codec context
	if e.codecCtx = astiav.AllocCodecContext(codec); e.codecCtx == nil {
		err = errors.New("astilibav: no codec context allocated")
		return
	}

	// Make sure the codec context is freed
	e.AddClose(e.codecCtx.Free)

	// Set shared context parameters
	if o.Ctx.GlobalHeader {
		e.codecCtx.SetFlags(e.codecCtx.Flags().Add(astiav.CodecContextFlagGlobalHeader))
	}
	if o.Ctx.ThreadCount != nil {
		e.codecCtx.SetThreadCount(*o.Ctx.ThreadCount)
	}
	if o.Ctx.ThreadType != nil {
		e.codecCtx.SetThreadType(*o.Ctx.ThreadType)
	}

	// Set media type-specific context parameters
	switch o.Ctx.MediaType {
	case astiav.MediaTypeAudio:
		e.codecCtx.SetBitRate(int64(o.Ctx.BitRate))
		e.codecCtx.SetChannelLayout(o.Ctx.ChannelLayout)
		e.codecCtx.SetChannels(o.Ctx.Channels)
		e.codecCtx.SetSampleFormat(o.Ctx.SampleFormat)
		e.codecCtx.SetSampleRate(o.Ctx.SampleRate)
	case astiav.MediaTypeVideo:
		e.codecCtx.SetBitRate(int64(o.Ctx.BitRate))
		e.codecCtx.SetFramerate(o.Ctx.FrameRate)
		e.codecCtx.SetGopSize(o.Ctx.GopSize)
		e.codecCtx.SetHeight(o.Ctx.Height)
		e.codecCtx.SetPixelFormat(o.Ctx.PixelFormat)
		e.codecCtx.SetSampleAspectRatio(o.Ctx.SampleAspectRatio)
		e.codecCtx.SetTimeBase(o.Ctx.TimeBase)
		e.codecCtx.SetWidth(o.Ctx.Width)
	default:
		err = fmt.Errorf("astilibav: encoder doesn't handle %s media type", o.Ctx.MediaType)
		return
	}

	// Dictionary
	var dict *astiav.Dictionary
	if o.Ctx.Dictionary != nil {
		// Parse dict
		if dict, err = o.Ctx.Dictionary.parse(); err != nil {
			err = fmt.Errorf("astilibav: parsing dict failed: %w", err)
			return
		}

		// Make sure the dictionary is freed
		defer dict.Free()
	}

	// Open codec
	if err = e.codecCtx.Open(codec, dict); err != nil {
		err = fmt.Errorf("astilibav: opening codec failed: %w", err)
		return
	}
	return
}

func (e *Encoder) addStats() {
	// Get stats
	ss := e.c.Stats()
	ss = append(ss, e.d.stats()...)
	ss = append(ss,
		astikit.StatOptions{
			Handler: e.statIncomingRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
		},
		astikit.StatOptions{
			Handler: e.statProcessedRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
		},
	)

	// Add stats
	e.BaseNode.AddStats(ss...)
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
		// Make sure to flush the encoder
		defer e.flush()

		// Make sure to stop the chan properly
		defer e.c.Stop()

		// Start chan
		e.c.Start(e.Context())
	})
}

func (e *Encoder) flush() {
	e.encode(nil, nil)
}

// HandleFrame implements the FrameHandler interface
func (e *Encoder) HandleFrame(p FrameHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	e.DoWhenUnclosed(func() {
		// Increment incoming rate
		e.statIncomingRate.Add(1)

		// Copy frame
		f := e.fp.get()
		if err := f.Ref(p.Frame); err != nil {
			emitError(e, e.eh, err, "refing frame")
			return
		}

		// Add to chan
		e.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			e.DoWhenUnclosed(func() {
				// Handle pause
				defer e.HandlePause()

				// Make sure to close frame
				defer e.fp.put(f)

				// Increment processed rate
				e.statProcessedRate.Add(1)

				// Encode
				e.encode(f, p.Descriptor)
			})
		})
	})
}

func (e *Encoder) encode(f *astiav.Frame, d Descriptor) {
	// Reset frame attributes
	if f != nil {
		switch e.codecCtx.MediaType() {
		case astiav.MediaTypeVideo:
			f.SetKeyFrame(false)
			f.SetPictureType(astiav.PictureTypeNone)
		}
	}

	// Send frame to encoder
	if err := e.codecCtx.SendFrame(f); err != nil {
		emitError(e, e.eh, err, "sending frame")
		return
	}

	// Loop
	for {
		// Receive pkt
		if stop := e.receivePkt(d); stop {
			return
		}
	}
}

func (e *Encoder) receivePkt(d Descriptor) (stop bool) {
	// Get pkt from pool
	pkt := e.pp.get()
	defer e.pp.put(pkt)

	// Receive pkt
	if err := e.codecCtx.ReceivePacket(pkt); err != nil {
		if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
			emitError(e, e.eh, err, "receiving packet")
		}
		stop = true
		return
	}

	// Get descriptor
	if d == nil && e.previousDescriptor == nil {
		e.eh.Emit(astiencoder.EventError(e, errors.New("astilibav: no valid descriptor")))
		return
	} else if d == nil {
		d = e.previousDescriptor
	} else {
		e.previousDescriptor = d
	}

	// Set pkt duration based on framerate
	if f := e.codecCtx.Framerate(); f.Num() > 0 {
		pkt.SetDuration(astiav.RescaleQ(int64(1e9/f.ToDouble()), nanosecondRational, d.TimeBase()))
	}

	// Rescale timestamps
	pkt.RescaleTs(d.TimeBase(), e.codecCtx.TimeBase())

	// Dispatch pkt
	e.d.dispatch(pkt, newEncoderDescriptor(e.codecCtx))
	return
}

// AddStream adds a stream based on the codec ctx
func (e *Encoder) AddStream(formatCtx *astiav.FormatContext) (o *astiav.Stream, err error) {
	// Add stream
	o = AddStream(formatCtx)

	// Set codec parameters
	if err = o.CodecParameters().FromCodecContext(e.codecCtx); err != nil {
		err = fmt.Errorf("astilibav: getting codec parameters from codec context failed: %w", err)
		return
	}

	// Set other attributes
	o.SetTimeBase(e.codecCtx.TimeBase())
	return
}

// FrameSize returns the encoder frame size
func (e *Encoder) FrameSize() int {
	return e.codecCtx.FrameSize()
}

type encoderDescriptor struct {
	codecCtx *astiav.CodecContext
}

func newEncoderDescriptor(codecCtx *astiav.CodecContext) *encoderDescriptor {
	return &encoderDescriptor{codecCtx: codecCtx}
}

// TimeBase implements the Descriptor interface
func (d *encoderDescriptor) TimeBase() astiav.Rational {
	return d.codecCtx.TimeBase()
}
