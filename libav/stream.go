package astilibav

import (
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
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
		err = errors.Wrapf(newAvError(ret), "astilibav: avcodec.AvcodecParametersCopy from %+v to %+v failed", i.CodecParameters(), o.CodecParameters())
		return
	}

	// Reset codec tag as shown in https://github.com/FFmpeg/FFmpeg/blob/n4.0.2/doc/examples/remuxing.c#L122
	o.CodecParameters().SetCodecTag(0)
	return
}
