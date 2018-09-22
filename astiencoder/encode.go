package main

import (
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

func (b *builder) addEncodeToWorkflow(name string, cfg astiencoder.JobOperation, bd *buildData, ois []openedInput, oos []openedOutput) (err error) {
	// Loop through inputs
	for _, i := range ois {
		// Loop through streams
		for iIdx, is := range i.ctxFormat.Streams() {
			// Only process some media types
			if is.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_AUDIO &&
				is.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_VIDEO {
				continue
			}

			// Add demuxer as root node of the workflow
			bd.w.AddChild(i.d)

			// Get decoder
			var d *astilibav.Decoder
			var okD, okS bool
			if _, okD = bd.decoders[i.d]; okD {
				d, okS = bd.decoders[i.d][is]
			} else {
				bd.decoders[i.d] = make(map[*avformat.Stream]*astilibav.Decoder)
			}

			// Decoder doesn't exist
			if !okD || !okS {
				// Create decoder
				if d, err = astilibav.NewDecoder(is, bd.w.EmitEventFunc(), bd.w.Closer()); err != nil {
					err = errors.Wrapf(err, "main: creating decoder for stream #%d of %s failed", iIdx, i.c.URL)
					return
				}

				// Connect demuxer to decoder
				i.d.Connect(is, d)

				// Index decoder
				bd.decoders[i.d][is] = d
			}
		}
	}
	return
}
