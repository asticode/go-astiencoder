package main

import (
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

func (b *builder) addRemuxToWorkflow(w *astiencoder.Workflow, ois []openedInput, oos []openedOutput) (err error) {
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
			w.AddChild(i.d)

			// Loop through outputs
			for _, o := range oos {
				// Clone stream
				var os *avformat.Stream
				if os, err = o.m.CloneStream(is); err != nil {
					err = errors.Wrapf(err, "main: cloning stream %+v of %s failed", is, i.ctxFormat.Filename())
					return
				}

				// Create transmuxer
				t := newTransmuxer()

				// Connect transmuxer to muxer
				t.connect(o.m)

				// Connect demuxer to transmuxer
				i.d.Connect(is, os, t)
			}
		}
	}
	return
}
