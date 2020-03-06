package astilibav

import (
	"fmt"
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

// NewContextFromStream creates a new context from a stream
func NewContextFromStream(s *avformat.Stream) Context {
	ctxCodec := (*avcodec.Context)(unsafe.Pointer(s.Codec()))
	return Context{
		// Shared
		BitRate:   ctxCodec.BitRate(),
		CodecID:   s.CodecParameters().CodecId(),
		CodecType: s.CodecParameters().CodecType(),
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
