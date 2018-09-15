package main

import (
	"fmt"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/go-astilog"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

type builder struct{}

func newBuilder() *builder {
	return &builder{}
}

type openedInput struct {
	ctxFormat *avformat.Context
	d         *astilibav.Demuxer
	i         astiencoder.JobInput
}

type openedOutput struct {
	ctxFormat *avformat.Context
	m         *astilibav.Muxer
	o         astiencoder.JobOutput
}

// BuildWorkflow implements the astiencoder.WorkflowBuilder interface
func (b *builder) BuildWorkflow(j astiencoder.Job, w *astiencoder.Workflow) (err error) {
	// Create opener
	o := astilibav.NewOpener(w.Closer())

	// No inputs
	if len(j.Inputs) == 0 {
		err = errors.New("main: no inputs provided")
		return
	}

	// Open inputs
	var is map[string]openedInput
	if is, err = b.openInputs(j, o, w.EmitEventFunc(), w.Closer()); err != nil {
		err = errors.Wrap(err, "main: opening inputs failed")
		return
	}

	// No outputs
	if len(j.Outputs) == 0 {
		err = errors.New("main: no outputs provided")
		return
	}

	// Open outputs
	var os map[string]openedOutput
	if os, err = b.openOutputs(j, o, w.EmitEventFunc()); err != nil {
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
		if err = b.addOperationToWorkflow(w, n, o, is, os); err != nil {
			err = errors.Wrapf(err, "main: adding operation %s with conf %+v to workflow failed", n, o)
			return
		}
	}
	return
}

func (b *builder) openInputs(j astiencoder.Job, o *astilibav.Opener, e astiencoder.EmitEventFunc, c *astiencoder.Closer) (is map[string]openedInput, err error) {
	// Loop through inputs
	is = make(map[string]openedInput)
	for n, i := range j.Inputs {
		// Open
		var ctxFormat *avformat.Context
		if ctxFormat, err = o.OpenInput(n, i); err != nil {
			err = errors.Wrapf(err, "main: opening input %s with conf %+v failed", n, i)
			return
		}

		// Index
		is[n] = openedInput{
			ctxFormat: ctxFormat,
			d:         astilibav.NewDemuxer(ctxFormat, e, c, 10),
			i:         i,
		}
	}
	return
}

func (b *builder) openOutputs(j astiencoder.Job, o *astilibav.Opener, e astiencoder.EmitEventFunc) (os map[string]openedOutput, err error) {
	// Loop through outputs
	os = make(map[string]openedOutput)
	for n, out := range j.Outputs {
		// Open
		var ctxFormat *avformat.Context
		if ctxFormat, err = o.OpenOutput(n, out); err != nil {
			err = errors.Wrapf(err, "main: opening output %s with conf %+v failed", n, out)
			return
		}

		// Index
		os[n] = openedOutput{
			ctxFormat: ctxFormat,
			m:         astilibav.NewMuxer(ctxFormat, e),
			o:         out,
		}
	}
	return
}

func (b *builder) addOperationToWorkflow(w *astiencoder.Workflow, name string, o astiencoder.JobOperation, ois map[string]openedInput, oos map[string]openedOutput) (err error) {
	// No inputs
	if len(o.Inputs) == 0 {
		err = errors.New("main: no operation inputs provided")
		return
	}

	// Loop through inputs
	var is []openedInput
	for _, pi := range o.Inputs {
		// Retrieve opened input
		i, ok := ois[pi.Name]
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
		o, ok := oos[po.Name]
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
		err = b.addRemuxToWorkflow(w, is, os)
	default:
		astilog.Warnf("main: unhandled job operation type %s", o.Type)
	}
	return
}
