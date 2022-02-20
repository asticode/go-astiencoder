package astilibav

import (
	"fmt"

	"github.com/asticode/go-astiav"
)

type Stream struct {
	CodecParameters *astiav.CodecParameters
	Ctx             Context
	ID              int
	Index           int
}

// AddStream adds a stream to the format ctx
func AddStream(formatCtx *astiav.FormatContext) *astiav.Stream {
	return formatCtx.NewStream(nil)
}

// CloneStream clones a stream and add it to the format ctx
func CloneStream(i *Stream, formatCtx *astiav.FormatContext) (o *astiav.Stream, err error) {
	// Add stream
	o = AddStream(formatCtx)

	// Copy codec parameters
	if err = i.CodecParameters.Copy(o.CodecParameters()); err != nil {
		err = fmt.Errorf("astilibav: copying codec parameters failed: %w", err)
		return
	}

	// Reset codec tag as shown in https://github.com/FFmpeg/FFmpeg/blob/n4.1.1/doc/examples/remuxing.c#L122
	o.CodecParameters().SetCodecTag(0)
	return
}
