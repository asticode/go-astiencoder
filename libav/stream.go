package astilibav

import (
	"fmt"

	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
)

// AddStream adds a stream to the format ctx
func AddStream(ctxFormat *avformat.Context) *avformat.Stream {
	return ctxFormat.AvformatNewStream(nil)
}

// CloneStream clones a stream and add it to the format ctx
func CloneStream(i *avformat.Stream, ctxFormat *avformat.Context) (o *avformat.Stream, err error) {
	// Add stream
	o = AddStream(ctxFormat)

	// Copy codec parameters
	if ret := avcodec.AvcodecParametersCopy(o.CodecParameters(), i.CodecParameters()); ret < 0 {
		err = fmt.Errorf("astilibav: avcodec.AvcodecParametersCopy from %+v to %+v failed: %w", i.CodecParameters(), o.CodecParameters(), NewAvError(ret))
		return
	}

	// Reset codec tag as shown in https://github.com/FFmpeg/FFmpeg/blob/n4.1.1/doc/examples/remuxing.c#L122
	o.CodecParameters().SetCodecTag(0)
	return
}
