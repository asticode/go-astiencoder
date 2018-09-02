package astilibav

import (
	"context"
	"io"

	"github.com/asticode/go-astiencoder"
	"github.com/pkg/errors"
)

// DefaultJobHandler allows creating the default libav job handler
var DefaultJobHandler = func(ctx context.Context, j astiencoder.Job, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) (io.Closer, error) {
	// Create closer
	c := astiencoder.NewCloser()

	// Create opener
	o := NewOpener(e, t)

	// Create demuxer
	d := NewDemuxer(e, t)

	// Connect the demuxer to the opener
	o.AddHandler(d.Demux)

	// Open
	if err := o.Open(ctx, j.URL); err != nil {
		return c, errors.Wrapf(err, "astilibav: opening job %+v failed", j)
	}
	return c, nil
}
