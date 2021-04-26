package astilibav

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
)

// Context represents parameters of an audio or a video context
type Context struct {
	// Shared
	BitRate      int
	CodecID      avcodec.CodecId
	CodecName    string
	CodecType    avcodec.MediaType
	Dict         *Dict
	GlobalHeader bool
	Index        int
	ThreadCount  *int
	TimeBase     avutil.Rational

	// Audio
	ChannelLayout uint64
	Channels      int
	SampleFmt     avcodec.AvSampleFormat
	SampleRate    int

	// Video
	FrameRate         avutil.Rational
	GopSize           int
	Height            int
	PixelFormat       avutil.PixelFormat
	SampleAspectRatio avutil.Rational
	Width             int
}

func (ctx Context) Descriptor() Descriptor {
	return NewDescriptor(ctx.TimeBase)
}

func (ctx Context) String() string {
	// Shared
	var ss []string
	ss = append(ss, "index: "+strconv.Itoa(ctx.Index))
	if ctx.CodecType >= 0 {
		var m string
		switch ctx.CodecType {
		case avutil.AVMEDIA_TYPE_ATTACHMENT:
			m = "attachment"
		case avutil.AVMEDIA_TYPE_AUDIO:
			m = "audio"
		case avutil.AVMEDIA_TYPE_DATA:
			m = "data"
		case avutil.AVMEDIA_TYPE_NB:
			m = "nb"
		case avutil.AVMEDIA_TYPE_SUBTITLE:
			m = "subtitle"
		case avutil.AVMEDIA_TYPE_VIDEO:
			m = "video"
		default:
			m = "unknown"
		}
		ss = append(ss, "codec type: "+m)
	}
	if ctx.BitRate > 0 {
		ss = append(ss, "bitrate: "+strconv.Itoa(ctx.BitRate))
	}
	if ctx.CodecName != "" {
		ss = append(ss, "codec name: "+ctx.CodecName)
	} else if ctx.CodecID > 0 {
		if d := avcodec.AvcodecDescriptorGet(ctx.CodecID); d != nil {
			ss = append(ss, "codec: "+d.Name())
		} else {
			ss = append(ss, "codec id: "+strconv.Itoa(int(ctx.CodecID)))
		}
	}
	if ctx.TimeBase.ToDouble() > 0 {
		ss = append(ss, "timebase: "+ctx.TimeBase.String())
	}

	// Switch on codec type
	switch ctx.CodecType {
	case avutil.AVMEDIA_TYPE_AUDIO:
		ss = append(ss, "channel layout: ", avutil.AvGetChannelLayoutString(ctx.Channels, ctx.ChannelLayout))
		if ctx.SampleFmt >= 0 {
			ss = append(ss, "sample fmt: "+avutil.AvGetSampleFmtName(int(ctx.SampleFmt)))
		}
		if ctx.SampleRate > 0 {
			ss = append(ss, "sample rate: "+strconv.Itoa(ctx.SampleRate))
		}
	case avutil.AVMEDIA_TYPE_VIDEO:
		if ctx.Height > 0 && ctx.Width > 0 {
			ss = append(ss, "video size: "+strconv.Itoa(ctx.Width)+"x"+strconv.Itoa(ctx.Height))
		}
		if ctx.PixelFormat >= 0 {
			ss = append(ss, "pixel format: "+avutil.AvGetPixFmtName(ctx.PixelFormat))
		}
		if ctx.SampleAspectRatio.ToDouble() > 0 {
			ss = append(ss, "sample aspect ratio: "+ctx.SampleAspectRatio.String())
		}
		if ctx.FrameRate.ToDouble() > 0 {
			ss = append(ss, "framerate: "+ctx.FrameRate.String())
		}
		if ctx.GopSize > 0 {
			ss = append(ss, "gop size: "+strconv.Itoa(ctx.GopSize))
		}
	}
	return strings.Join(ss, " - ")
}

type OutputContexter interface {
	OutputCtx() Context
}

// NewContextFromStream creates a new context from a stream
func NewContextFromStream(s *avformat.Stream) Context {
	ctxCodec := (*avcodec.Context)(unsafe.Pointer(s.Codec()))
	return Context{
		// Shared
		BitRate:   ctxCodec.BitRate(),
		CodecID:   s.CodecParameters().CodecId(),
		CodecType: s.CodecParameters().CodecType(),
		Index:     s.Index(),
		TimeBase:  s.TimeBase(),

		// Audio
		ChannelLayout: ctxCodec.ChannelLayout(),
		Channels:      ctxCodec.Channels(),
		SampleFmt:     ctxCodec.SampleFmt(),
		SampleRate:    ctxCodec.SampleRate(),

		// Video
		FrameRate:         streamFrameRate(s),
		GopSize:           ctxCodec.GopSize(),
		Height:            ctxCodec.Height(),
		PixelFormat:       ctxCodec.PixFmt(),
		SampleAspectRatio: s.SampleAspectRatio(),
		Width:             ctxCodec.Width(),
	}
}

func streamFrameRate(s *avformat.Stream) avutil.Rational {
	if v := s.AvgFrameRate(); v.Num() > 0 {
		return s.AvgFrameRate()
	}
	return s.RFrameRate()
}

func (ctx Context) validWithCodec(c *avcodec.Codec) (err error) {
	switch ctx.CodecType {
	case avutil.AVMEDIA_TYPE_AUDIO:
		// Check channel layout
		var correct bool
		if len(c.ChannelLayouts()) > 0 {
			for _, v := range c.ChannelLayouts() {
				if v == ctx.ChannelLayout {
					correct = true
					break
				}
			}
			if !correct {
				err = fmt.Errorf("astilibav: channel layout %d is not valid with chosen codec", ctx.ChannelLayout)
				return
			}
		}

		// Check sample fmt
		if len(c.SampleFmts()) > 0 {
			correct = false
			for _, v := range c.SampleFmts() {
				if v == ctx.SampleFmt {
					correct = true
					break
				}
			}
			if !correct {
				err = fmt.Errorf("astilibav: sample fmt %v is not valid with chosen codec", ctx.SampleFmt)
				return
			}
		}

		// Check sample rate
		if len(c.SupportedSamplerates()) > 0 {
			correct = false
			for _, v := range c.SupportedSamplerates() {
				if v == ctx.SampleRate {
					correct = true
					break
				}
			}
			if !correct {
				err = fmt.Errorf("astilibav: sample rate %d is not valid with chosen codec", ctx.SampleRate)
				return
			}
		}
	}
	return
}
