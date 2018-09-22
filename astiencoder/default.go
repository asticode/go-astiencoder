package main

import (
	"fmt"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

func (b *builder) addDefaultOperation(name string, cfg astiencoder.JobOperation, bd *buildData, ois []openedInput, oos []openedOutput) (err error) {
	// Get media type whitelist
	var w map[avcodec.MediaType]bool
	switch cfg.Format {
	case astiencoder.JobOperationFormatJpeg:
		w = map[avcodec.MediaType]bool{avcodec.AVMEDIA_TYPE_VIDEO: true}
	default:
		w = map[avcodec.MediaType]bool{
			avcodec.AVMEDIA_TYPE_AUDIO: true,
			avcodec.AVMEDIA_TYPE_VIDEO: true,
		}
	}

	// Loop through inputs
	for _, i := range ois {
		// Loop through streams
		for iIdx, is := range i.ctxFormat.Streams() {
			// Only process some media types
			if _, ok := w[is.CodecParameters().CodecType()]; !ok {
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
				if d, err = astilibav.NewDecoder(is, bd.w.EmitEventFunc(), bd.w.Closer(), 1); err != nil {
					err = errors.Wrapf(err, "main: creating decoder for stream #%d of %s failed", iIdx, i.c.URL)
					return
				}

				// Connect demuxer to decoder
				i.d.Connect(is, d)

				// Index decoder
				bd.decoders[i.d][is] = d
			}

			// Create encoder

			// Connect decoder to encoder

			// Loop through outputs
			for _, o := range oos {
				// Handler has not been created yet
				if o.h == nil {
					switch o.c.Type {
					case astiencoder.JobOutputTypePktDump:
						// Create pkt dumper
						if o.h, err = astilibav.NewPktDumper(o.c.URL, astilibav.PktDumpFile, map[string]interface{}{"input": i.name}, bd.w.EmitEventFunc()); err != nil {
							err = errors.Wrapf(err, "main: creating pkt dumper for output %s with conf %+v failed", o.name, cfg)
							return
						}
					default:
						err = fmt.Errorf("main: invalid job output type %s when creating output handler", o.c.Type)
						return
					}
				}

				// Connect encoder to handler
			}
		}
	}
	return
}
