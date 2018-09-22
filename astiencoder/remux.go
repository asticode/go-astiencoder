package main

import (
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

func (b *builder) addRemuxToWorkflow(name string, cfg astiencoder.JobOperation, bd *buildData, ois []openedInput, oos []openedOutput) (err error) {
	// Loop through inputs
	for _, i := range ois {
		// Loop through streams
		for _, is := range i.ctxFormat.Streams() {
			// Only process some media types
			if is.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_AUDIO &&
				is.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_SUBTITLE &&
				is.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_VIDEO {
				continue
			}

			// Add demuxer as root node of the workflow
			bd.w.AddChild(i.d)

			// Loop through outputs
			for _, o := range oos {
				// Clone stream
				var os *avformat.Stream
				if os, err = astilibav.CloneStream(is, o.ctxFormat); err != nil {
					err = errors.Wrapf(err, "main: cloning stream %+v of %s failed", is, i.ctxFormat.Filename())
					return
				}

				// Create transmuxer
				t := newTransmuxer(o.h, is, os)

				// Connect demuxer to transmuxer
				i.d.Connect(is, t)
			}
		}
	}
	return
}
