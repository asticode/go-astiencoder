package main

import (
	"fmt"

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
	// No inputs
	if len(o.Inputs) == 0 {
		err = errors.New("main: no operation inputs provided")
		return
	}

	// Loop through inputs
	var is []openedInput
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
	var os []openedOutput
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

	// Switch on operation type
	switch o.Type {
	case astiencoder.JobOperationTypeRemux:
		err = b.addRemuxOperation(name, o, bd, is, os)
	default:
		err = b.addDefaultOperation(name, o, bd, is, os)
	}
	return
}
