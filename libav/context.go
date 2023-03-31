package astilibav

import (
	"strconv"
	"strings"

	"github.com/asticode/go-astiav"
)

// Context represents parameters of an audio or a video context
type Context struct {
	// Shared
	BitRate      int64
	CodecID      astiav.CodecID
	CodecName    string
	Dictionary   *Dictionary
	GlobalHeader bool
	Index        int
	MediaType    astiav.MediaType
	ThreadCount  *int
	ThreadType   *astiav.ThreadType
	TimeBase     astiav.Rational

	// Audio
	ChannelLayout *astiav.ChannelLayout
	Channels      int
	FrameSize     int
	SampleFormat  astiav.SampleFormat
	SampleRate    int

	// Video
	ColorRange        astiav.ColorRange
	FrameRate         astiav.Rational
	GopSize           int
	Height            int
	PixelFormat       astiav.PixelFormat
	Rotation          float64
	SampleAspectRatio astiav.Rational
	Width             int
}

func (ctx Context) Descriptor() Descriptor {
	return NewDescriptor(ctx.TimeBase)
}

func (ctx Context) String() string {
	// Shared
	var ss []string
	ss = append(ss, "index: "+strconv.Itoa(ctx.Index))
	ss = append(ss, "media type: "+ctx.MediaType.String())
	if ctx.BitRate > 0 {
		ss = append(ss, "bitrate: "+strconv.FormatInt(ctx.BitRate, 10))
	}
	if ctx.CodecName != "" {
		ss = append(ss, "codec name: "+ctx.CodecName)
	} else if ctx.CodecID > 0 {
		ss = append(ss, "codec id: "+ctx.CodecID.String())
	}
	if ctx.TimeBase.ToDouble() > 0 {
		ss = append(ss, "timebase: "+ctx.TimeBase.String())
	}

	// Switch on media type
	switch ctx.MediaType {
	case astiav.MediaTypeAudio:
		ss = append(ss, "channel layout: ", ctx.ChannelLayout.String())
		if ctx.SampleFormat >= 0 {
			ss = append(ss, "sample fmt: "+ctx.SampleFormat.String())
		}
		if ctx.SampleRate > 0 {
			ss = append(ss, "sample rate: "+strconv.Itoa(ctx.SampleRate))
		}
	case astiav.MediaTypeVideo:
		if ctx.Height > 0 && ctx.Width > 0 {
			ss = append(ss, "video size: "+strconv.Itoa(ctx.Width)+"x"+strconv.Itoa(ctx.Height))
		}
		if ctx.PixelFormat >= 0 {
			ss = append(ss, "pixel format: "+ctx.PixelFormat.String())
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
		if ctx.Rotation != 0 {
			ss = append(ss, "rotation: "+strconv.FormatFloat(ctx.Rotation, 'f', 2, 64))
		}
	}
	return strings.Join(ss, " - ")
}

type OutputContexter interface {
	OutputCtx() Context
}

func NewContextFromStream(s *astiav.Stream) (ctx Context) {
	// Get codec parameters
	cp := s.CodecParameters()

	// Create context
	ctx = Context{
		// Shared
		BitRate:   cp.BitRate(),
		CodecID:   cp.CodecID(),
		Index:     s.Index(),
		MediaType: cp.MediaType(),
		TimeBase:  s.TimeBase(),

		// Audio
		ChannelLayout: cp.ChannelLayout(),
		Channels:      cp.Channels(),
		FrameSize:     cp.FrameSize(),
		SampleFormat:  cp.SampleFormat(),
		SampleRate:    cp.SampleRate(),

		// Video
		ColorRange:        cp.ColorRange(),
		FrameRate:         streamFrameRate(s),
		Height:            cp.Height(),
		PixelFormat:       cp.PixelFormat(),
		SampleAspectRatio: s.SampleAspectRatio(),
		Width:             cp.Width(),
	}

	// Get display matrix side data
	if sd := s.SideData(astiav.PacketSideDataTypeDisplaymatrix); len(sd) > 0 {
		if dm, err := astiav.NewDisplayMatrixFromBytes(sd); err == nil {
			ctx.Rotation = dm.Rotation()
		}
	}
	return
}

func streamFrameRate(s *astiav.Stream) astiav.Rational {
	if v := s.AvgFrameRate(); v.Num() > 0 {
		return s.AvgFrameRate()
	}
	return s.RFrameRate()
}
