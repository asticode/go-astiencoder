package main

import (
	"context"
	"io"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/go-astilog"
	"github.com/pkg/errors"
)

type handler struct {
	d *astilibav.Demuxer
	o *astilibav.Opener
}

func newHandler() *handler {
	return &handler{}
}

// HandleJob implements the astiencoder.JobHandler interface
func (h *handler) HandleJob(ctx context.Context, j astiencoder.Job, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) (io.Closer, error) {
	// Create closer
	c := astiencoder.NewCloser()

	// Create opener
	h.o = astilibav.NewOpener(c, e, t)

	// Create demuxer
	h.d = astilibav.NewDemuxer(e, t)

	// Connect the demuxer to the opener
	h.o.AddHandleResultFunc(h.d.Demux)

	// Open
	if err := h.o.Open(ctx, j.URL); err != nil {
		return c, errors.Wrap(err, "main: open failed")
	}
	return c, nil
}

// HandleCmd implements the astiencoder.CmdHandler interface
func (h *handler) HandleCmd(c astiencoder.Cmd) {
	astilog.Debugf("main: TODO: handle cmd %+v", c)
}
