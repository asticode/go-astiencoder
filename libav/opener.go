package astilibav

import (
	"context"

	"github.com/pkg/errors"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avformat"
)

// Opener represents an object capable of opening inputs and outputs
type Opener struct {
	c *astiencoder.Closer
}

// NewOpener creates a new Opener
func NewOpener(c *astiencoder.Closer) *Opener {
	return &Opener{
		c: c,
	}
}

// OpenInput opens an input
func (o *Opener) OpenInput(ctx context.Context, name string, i astiencoder.JobInput) (ctxFormat *avformat.Context, err error) {
	// Open input
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ret := avformat.AvformatOpenInput(&ctxFormat, i.URL, nil, nil); ret < 0 {
			return errors.Wrapf(newAvErr(ret), "astilibav: avformat.AvformatOpenInput on input %s with conf %+v failed", name, i)
		}
		return nil
	}); err != nil {
		return
	}

	// Retrieve stream information
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ret := ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
			return errors.Wrapf(newAvErr(ret), "astilibav: ctxFormat.AvformatFindStreamInfo on input %s with conf %+v failed", name, i)
		}
		return nil
	}); err != nil {
		return
	}

	// Make sure the format ctx is properly closed
	o.c.AddCloseFunc(func() error {
		avformat.AvformatCloseInput(ctxFormat)
		return nil
	})
	return
}
