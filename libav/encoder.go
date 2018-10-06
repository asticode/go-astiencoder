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
	e                astiencoder.EmitEventFunc
	prev             Descriptor
	q                *astisync.CtxQueue
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// NewEncoder creates a new encoder
func NewEncoder(ctxCodec *avcodec.Context, prev Descriptor, ee astiencoder.EmitEventFunc, c *astiencoder.Closer) (e *Encoder) {
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
		prev:             prev,
		q:                astisync.NewCtxQueue(),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	e.addStats()
	return
}

// EncoderOptions represents encoder options
type EncoderOptions struct {
	// Mandatory options
	BitRate           int
	CodecID           avcodec.CodecId
	CodecName         string
	CodecType         avcodec.MediaType
	FrameRate         avutil.Rational
	GopSize           int
	Height            int
	PixelFormat       avutil.PixelFormat
	SampleAspectRatio avutil.Rational
	TimeBase          avutil.Rational
	Width             int

	// Optional options
	Dict        string
	ThreadCount *int
}

// NewEncoderFromOptions creates a new encoder based on options
func NewEncoderFromOptions(o EncoderOptions, prev Descriptor, e astiencoder.EmitEventFunc, c *astiencoder.Closer) (_ *Encoder, err error) {
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
		err = errors.New("astilibav: no context allocated")
		return
	}

	// Set global context parameters
	ctxCodec.SetFlags(ctxCodec.Flags() | avcodec.AV_CODEC_FLAG_GLOBAL_HEADER)
	if o.ThreadCount != nil {
		ctxCodec.SetThreadCount(*o.ThreadCount)
	}

	// Set media type-specific context parameters
	switch o.CodecType {
	case avutil.AVMEDIA_TYPE_VIDEO:
		ctxCodec.SetBitRate(int64(o.BitRate))
		ctxCodec.SetFramerate(o.FrameRate)
		ctxCodec.SetGopSize(o.GopSize)
		ctxCodec.SetHeight(o.Height)
		ctxCodec.SetPixFmt(o.PixelFormat)
		ctxCodec.SetSampleAspectRatio(o.SampleAspectRatio)
		ctxCodec.SetTimeBase(o.TimeBase)
		ctxCodec.SetWidth(o.Width)
	}

	// Dict
	var dict *avutil.Dictionary
	if len(o.Dict) > 0 {
		// Parse dict
		if ret := avutil.AvDictParseString(&dict, o.Dict, "=", ",", 0); ret < 0 {
			err = errors.Wrapf(newAvError(ret), "astilibav: avutil.AvDictParseString on %s failed", o.Dict)
			return
		}

		// Make sure the dict is freed
		defer avutil.AvDictFree(&dict)
	}

	// Open codec
	if ret := ctxCodec.AvcodecOpen2(cdc, &dict); ret < 0 {
		err = errors.Wrap(newAvError(ret), "astilibav: d.ctxCodec.AvcodecOpen2 failed")
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
	return NewEncoder(ctxCodec, prev, e, c), nil
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

		// Make sure to flush the encoder
		defer e.flush()

		// Make sure to stop the queue properly
		defer e.q.Stop()

		// Start queue
		e.q.Start(func(p interface{}) {
			// Handle pause
			defer e.HandlePause()

			// Assert payload
			f := p.(*avutil.Frame)

			// Increment incoming rate
			e.statIncomingRate.Add(1)

			// Encode
			e.encode(f)
		})
	})
}

func (e *Encoder) flush() {
	e.encode(nil)
}

func (e *Encoder) encode(f *avutil.Frame) {
	// Send frame to encoder
	e.statWorkRatio.Add(true)
	if ret := avcodec.AvcodecSendFrame(e.ctxCodec, f); ret < 0 {
		e.statWorkRatio.Done(true)
		emitAvError(e.e, ret, "avcodec.AvcodecSendFrame failed")
		return
	}
	e.statWorkRatio.Done(true)

	// Loop
	for {
		// Receive pkt
		if stop := e.receivePkt(); stop {
			return
		}
	}
}

func (e *Encoder) receivePkt() (stop bool) {
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

	// Rescale timestamps
	pkt.AvPacketRescaleTs(e.prev.TimeBase(), e.ctxCodec.TimeBase())

	// Dispatch pkt
	e.d.dispatch(pkt)
	return
}

// HandleFrame implements the FrameHandler interface
func (e *Encoder) HandleFrame(f *avutil.Frame) {
	e.q.Send(f, true)
}

// AddStream adds a stream based on the codec ctx
func (e *Encoder) AddStream(ctxFormat *avformat.Context) (o *avformat.Stream, err error) {
	// Add stream
	o = AddStream(ctxFormat)

	// Set codec parameters
	if ret := avcodec.AvcodecParametersFromContext(o.CodecParameters(), e.ctxCodec); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: avcodec.AvcodecParametersFromContext from %+v to %+v failed", e.ctxCodec, o.CodecParameters())
		return
	}

	// Set other attributes
	o.SetTimeBase(e.ctxCodec.TimeBase())
	return
}

// TimeBase implements the Descriptor interface
func (e *Encoder) TimeBase() avutil.Rational {
	return e.ctxCodec.TimeBase()
}
