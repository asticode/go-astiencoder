package main

import (
	"fmt"
	"unsafe"

	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

type builder struct{}

func newBuilder() *builder {
	return &builder{}
}

type openedInput struct {
	c         astiencoder.JobInput
	ctxFormat *avformat.Context
	d         *astilibav.Demuxer
}

type openedOutput struct {
	c         astiencoder.JobOutput
	ctxFormat *avformat.Context
	h         nodePktHandler
}

type nodePktHandler interface {
	astiencoder.Node
	astilibav.PktHandler
}

type buildData struct {
	decoders map[*astilibav.Demuxer]map[*avformat.Stream]*astilibav.Decoder
	inputs   map[string]openedInput
	outputs  map[string]openedOutput
	w        *astiencoder.Workflow
}

func newBuildData(w *astiencoder.Workflow) *buildData {
	return &buildData{
		decoders: make(map[*astilibav.Demuxer]map[*avformat.Stream]*astilibav.Decoder),
		w:        w,
	}
}

// BuildWorkflow implements the astiencoder.WorkflowBuilder interface
func (b *builder) BuildWorkflow(j astiencoder.Job, w *astiencoder.Workflow) (err error) {
	// Create build data
	bd := newBuildData(w)

	// Create opener
	o := astilibav.NewOpener(w.Closer())

	// No inputs
	if len(j.Inputs) == 0 {
		err = errors.New("main: no inputs provided")
		return
	}

	// Open inputs
	if bd.inputs, err = b.openInputs(j, o, w.EmitEventFunc(), w.Closer()); err != nil {
		err = errors.Wrap(err, "main: opening inputs failed")
		return
	}

	// No outputs
	if len(j.Outputs) == 0 {
		err = errors.New("main: no outputs provided")
		return
	}

	// Open outputs
	if bd.outputs, err = b.openOutputs(j, o, w.EmitEventFunc(), w.Closer()); err != nil {
		err = errors.Wrap(err, "main: opening outputs failed")
		return
	}

	// No operations
	if len(j.Operations) == 0 {
		err = errors.New("main: no operations provided")
		return
	}

	// Loop through operations
	for n, o := range j.Operations {
		// Add operation to workflow
		if err = b.addOperationToWorkflow(n, o, bd); err != nil {
			err = errors.Wrapf(err, "main: adding operation %s with conf %+v to workflow failed", n, o)
			return
		}
	}
	return
}

func (b *builder) openInputs(j astiencoder.Job, o *astilibav.Opener, e astiencoder.EmitEventFunc, c *astiencoder.Closer) (is map[string]openedInput, err error) {
	// Loop through inputs
	is = make(map[string]openedInput)
	for n, cfg := range j.Inputs {
		// Open
		var ctxFormat *avformat.Context
		if ctxFormat, err = o.OpenInput(n, cfg); err != nil {
			err = errors.Wrapf(err, "main: opening input %s with conf %+v failed", n, cfg)
			return
		}

		// Index
		is[n] = openedInput{
			c:         cfg,
			ctxFormat: ctxFormat,
			d:         astilibav.NewDemuxer(ctxFormat, e, c, 10),
		}
	}
	return
}

func (b *builder) openOutputs(j astiencoder.Job, o *astilibav.Opener, e astiencoder.EmitEventFunc, c *astiencoder.Closer) (os map[string]openedOutput, err error) {
	// Loop through outputs
	os = make(map[string]openedOutput)
	for n, cfg := range j.Outputs {
		// Create output
		oo := openedOutput{
			c: cfg,
		}

		// Switch on type
		switch cfg.Type {
		case astiencoder.JobOutputTypePktDump:
			// This is a per-operation and per-input value since we may want to index the path by input name
			// The writer is created afterwards
		default:
			// Open
			if oo.ctxFormat, err = o.OpenOutput(n, cfg); err != nil {
				err = errors.Wrapf(err, "main: opening output %s with conf %+v failed", n, cfg)
				return
			}

			// Create muxer
			oo.h = astilibav.NewMuxer(oo.ctxFormat, e, c)
		}

		// Index
		os[n] = oo
	}
	return
}

type operationInput struct {
	c astiencoder.JobOperationInput
	o openedInput
}

type operationOutput struct {
	c astiencoder.JobOperationOutput
	o openedOutput
}

func (b *builder) addOperationToWorkflow(name string, o astiencoder.JobOperation, bd *buildData) (err error) {
	// Get operation inputs and outputs
	var ois []operationInput
	var oos []operationOutput
	if ois, oos, err = b.operationInputsOutputs(o, bd); err != nil {
		err = errors.Wrapf(err, "main: getting inputs and outputs of operation %s failed", name)
		return
	}

	// Loop through inputs
	for _, i := range ois {
		// Loop through streams
		for _, is := range i.o.ctxFormat.Streams() {
			// Only process a specific PID
			if i.c.PID != nil && is.Id() != *i.c.PID {
				continue
			}

			// Only process a specific media type
			if t := avutil.MediaTypeFromString(i.c.MediaType); t > -1 && is.CodecParameters().CodecType() != avcodec.MediaType(t) {
				continue
			}

			// Add demuxer as root node of the workflow
			bd.w.AddChild(i.o.d)

			// In case of copy we only want to connect the demuxer to the muxer through a transmuxer
			if o.Codec == astiencoder.JobOperationCodecCopy {
				// Create transmuxers
				if err = b.createTransmuxers(i, oos, is); err != nil {
					err = errors.Wrapf(err, "main: creating transmuxers for stream 0x%x(%d) of input %s failed", is.Id(), is.Id(), i.c.Name)
					return
				}
				continue
			}

			// Create decoder
			var d *astilibav.Decoder
			if d, err = b.createDecoder(bd, i, is); err != nil {
				err = errors.Wrapf(err, "main: creating decoder for stream 0x%x(%d) of input %s failed", is.Id(), is.Id(), i.c.Name)
				return
			}

			// Create encoder options
			eo := b.createEncoderOptions(is, o)

			// Create encoder
			var e *astilibav.Encoder
			if e, err = astilibav.NewEncoderFromOptions(eo, bd.w.EmitEventFunc(), bd.w.Closer(), 10); err != nil {
				err = errors.Wrapf(err, "main: creating encoder for stream 0x%x(%d) of input %s failed", is.Id(), is.Id(), i.c.Name)
				return
			}

			// Connect decoder to encoder
			d.Connect(e)

			// Loop through outputs
			for _, o := range oos {
				// Create pkt dumper
				if o.o.c.Type == astiencoder.JobOutputTypePktDump {
					if o.o.h, err = astilibav.NewPktDumper(o.o.c.URL, astilibav.PktDumpFile, map[string]interface{}{"input": i.c.Name}, bd.w.EmitEventFunc()); err != nil {
						err = errors.Wrapf(err, "main: creating pkt dumper for output %s with conf %+v failed", o.c.Name, o.c)
						return
					}
				}

				// Connect encoder to handler
				e.Connect(o.o.h)
			}
		}
	}
	return
}

func (b *builder) operationInputsOutputs(o astiencoder.JobOperation, bd *buildData) (is []operationInput, os []operationOutput, err error) {
	// No inputs
	if len(o.Inputs) == 0 {
		err = errors.New("main: no operation inputs provided")
		return
	}

	// Loop through inputs
	for _, pi := range o.Inputs {
		// Retrieve opened input
		i, ok := bd.inputs[pi.Name]
		if !ok {
			err = fmt.Errorf("main: opened input %s not found", pi.Name)
			return
		}

		// Append input
		is = append(is, operationInput{
			c: pi,
			o: i,
		})
	}

	// No outputs
	if len(o.Outputs) == 0 {
		err = errors.New("main: no operation outputs provided")
		return
	}

	// Loop through outputs
	for _, po := range o.Outputs {
		// Retrieve opened output
		o, ok := bd.outputs[po.Name]
		if !ok {
			err = fmt.Errorf("main: opened output %s not found", po.Name)
			return
		}

		// Append output
		os = append(os, operationOutput{
			c: po,
			o: o,
		})
	}
	return
}

func (b *builder) createTransmuxers(i operationInput, oos []operationOutput, is *avformat.Stream) (err error) {
	// Loop through outputs
	for _, o := range oos {
		// Clone stream
		var os *avformat.Stream
		if os, err = astilibav.CloneStream(is, o.o.ctxFormat); err != nil {
			err = errors.Wrapf(err, "main: cloning stream 0x%x(%d) of %s failed", is.Id(), is.Id(), i.c.Name)
			return
		}

		// Create transmuxer
		t := newTransmuxer(o.o.h, is, os)

		// Connect demuxer to transmuxer
		i.o.d.Connect(is, t)
	}
	return
}

func (b *builder) createDecoder(bd *buildData, i operationInput, is *avformat.Stream) (d *astilibav.Decoder, err error) {
	// Get decoder
	var okD, okS bool
	if _, okD = bd.decoders[i.o.d]; okD {
		d, okS = bd.decoders[i.o.d][is]
	} else {
		bd.decoders[i.o.d] = make(map[*avformat.Stream]*astilibav.Decoder)
	}

	// Decoder doesn't exist
	if !okD || !okS {
		// Create decoder
		if d, err = astilibav.NewDecoderFromCodecParams(is.CodecParameters(), bd.w.EmitEventFunc(), bd.w.Closer(), 10); err != nil {
			err = errors.Wrapf(err, "main: creating decoder for stream 0x%x(%d) of %s failed", is.Id(), is.Id(), i.c.Name)
			return
		}

		// Connect demuxer to decoder
		i.o.d.Connect(is, d)

		// Index decoder
		bd.decoders[i.o.d][is] = d
	}
	return
}

func (b *builder) createEncoderOptions(s *avformat.Stream, o astiencoder.JobOperation) (eo astilibav.EncoderOptions) {
	// Create options
	eo = astilibav.EncoderOptions{
		CodecID:   s.CodecParameters().CodecId(),
		CodecName: o.Codec,
		CodecType: s.CodecParameters().CodecType(),
	}

	// Get stream codec context
	ctxCodec := (*avcodec.Context)(unsafe.Pointer(s.Codec()))

	// Set frame rate
	eo.FrameRate = s.AvgFrameRate()
	if o.FrameRate != nil {
		eo.FrameRate = avutil.NewRational(o.FrameRate.Num, o.FrameRate.Den)
	}

	// Set time base
	eo.TimeBase = s.TimeBase()
	if o.TimeBase != nil {
		eo.TimeBase = avutil.NewRational(o.TimeBase.Num, o.TimeBase.Den)
	} else if o.FrameRate != nil {
		eo.TimeBase = avutil.NewRational(o.FrameRate.Den, o.FrameRate.Num)
	}

	// Set pixel format
	eo.PixelFormat = ctxCodec.PixFmt()
	if len(o.PixelFormat) > 0 {
		eo.PixelFormat = avutil.PixelFormatFromString(o.PixelFormat)
	} else if o.Codec == "mjpeg" {
		eo.PixelFormat = avutil.AV_PIX_FMT_YUVJ420P
	}

	// Set height
	eo.Height = ctxCodec.Height()
	if o.Height != nil {
		eo.Height = *o.Height
	}

	// Set width
	eo.Width = ctxCodec.Width()
	if o.Width != nil {
		eo.Width = *o.Width
	}
	return
}
