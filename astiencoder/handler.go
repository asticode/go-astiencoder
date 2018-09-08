package main

import (
	"context"
	"io"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/go-astilog"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

type handler struct{}

func newHandler() *handler {
	return &handler{}
}

type openedInput struct {
	ctxFormat *avformat.Context
	d         *astilibav.Demuxer
	i         astiencoder.JobInput
}

// HandleJob implements the astiencoder.JobHandler interface
func (h *handler) HandleJob(ctx context.Context, j astiencoder.Job, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) (io.Closer, error) {
	// Create closer
	c := astiencoder.NewCloser()

	// Create opener
	o := astilibav.NewOpener(c)

	// Loop through inputs
	var is = make(map[string]openedInput)
	for n, i := range j.Inputs {
		// Open
		ctxFormat, err := o.OpenInput(ctx, n, i)
		if err != nil {
			return c, errors.Wrapf(err, "main: opening input %s with conf %+v failed", n, i)
		}

		// Index
		is[n] = openedInput{
			ctxFormat: ctxFormat,
			d:         astilibav.NewDemuxer(c, e, t),
			i:         i,
		}
	}

	// TODO Prepare outputs

	// Loop through processes
	for n, p := range j.Processes {
		_ = n
		switch p.Type {
		case astiencoder.JobProcessTypeRemux:

		default:
			astilog.Warnf("main: unhandled job process type %s", p.Type)
		}
	}
	return c, nil
}

// HandleCmd implements the astiencoder.CmdHandler interface
func (h *handler) HandleCmd(c astiencoder.Cmd) {
	astilog.Debugf("main: TODO: handle cmd %+v", c)
}
