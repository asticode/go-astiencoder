package main

import (
	"fmt"

	"github.com/asticode/goav/avcodec"

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
	name      string
}

type openedOutput struct {
	c         astiencoder.JobOutput
	ctxFormat *avformat.Context
	h         nodePktHandler
	name      string
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
			name:      n,
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
			c:    cfg,
			name: n,
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

func (b *builder) addOperationToWorkflow(name string, o astiencoder.JobOperation, bd *buildData) (err error) {
	// Get operation inputs and outputs
	var ois []openedInput
	var oos []openedOutput
	if ois, oos, err = b.operationInputsOutputs(o, bd); err != nil {
		err = errors.Wrapf(err, "main: getting inputs and outputs of operation %s failed", name)
		return
	}

	// Get media type whitelist
	w := b.operationMediaTypes(o)

	// Loop through inputs
	for _, i := range ois {
		// Loop through streams
		for _, is := range i.ctxFormat.Streams() {
			// Only process some media types
			if _, ok := w[is.CodecParameters().CodecType()]; !ok {
				continue
			}

			// Add demuxer as root node of the workflow
			bd.w.AddChild(i.d)

			// In case of remux we only want to connect the demuxer to the muxer through a transmuxer
			if o.Type == astiencoder.JobOperationTypeRemux {
				// Create transmuxers
				if err = b.createTransmuxers(i, oos, is); err != nil {
					err = errors.Wrapf(err, "main: creating transmuxers for stream #%d of input %s failed", is.Index(), i.name)
					return
				}
				continue
			}

			// Create decoder
			var d *astilibav.Decoder
			if d, err = b.createDecoder(bd, i, is); err != nil {
				err = errors.Wrapf(err, "main: creating decoder for stream #%d of input %s failed", is.Index(), i.name)
				return
			}

			_ = d

			// TODO Create encoder

			// TODO Connect decoder to encoder

			// Loop through outputs
			for _, o := range oos {
				// Create pkt dumper
				if o.c.Type == astiencoder.JobOutputTypePktDump {
					if o.h, err = astilibav.NewPktDumper(o.c.URL, astilibav.PktDumpFile, map[string]interface{}{"input": i.name}, bd.w.EmitEventFunc()); err != nil {
						err = errors.Wrapf(err, "main: creating pkt dumper for output %s with conf %+v failed", o.name, o.c)
						return
					}
				}

				// TODO Connect encoder to handler
			}
		}
	}
	return
}

func (b *builder) operationInputsOutputs(o astiencoder.JobOperation, bd *buildData) (is []openedInput, os []openedOutput, err error) {
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
		is = append(is, i)
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
		os = append(os, o)
	}
	return
}

func (b *builder) operationMediaTypes(o astiencoder.JobOperation) (w map[avcodec.MediaType]bool) {
	switch {
	case o.Format == astiencoder.JobOperationFormatJpeg:
		w = map[avcodec.MediaType]bool{
			avcodec.AVMEDIA_TYPE_VIDEO: true,
		}
	case o.Type == astiencoder.JobOperationTypeRemux:
		w = map[avcodec.MediaType]bool{
			avcodec.AVMEDIA_TYPE_AUDIO: true,
			avcodec.AVMEDIA_TYPE_VIDEO: true,
		}
	default:
		w = map[avcodec.MediaType]bool{
			avcodec.AVMEDIA_TYPE_AUDIO:    true,
			avcodec.AVMEDIA_TYPE_SUBTITLE: true,
			avcodec.AVMEDIA_TYPE_VIDEO:    true,
		}
	}
	return
}

func (b *builder) createTransmuxers(i openedInput, oos []openedOutput, is *avformat.Stream) (err error) {
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
	return
}

func (b *builder) createDecoder(bd *buildData, i openedInput, is *avformat.Stream) (d *astilibav.Decoder, err error) {
	// Get decoder
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
			err = errors.Wrapf(err, "main: creating decoder for stream #%d of %s failed", is.Index(), i.c.URL)
			return
		}

		// Connect demuxer to decoder
		i.d.Connect(is, d)

		// Index decoder
		bd.decoders[i.d][is] = d
	}
	return
}
