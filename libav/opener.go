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
func (o *Opener) OpenInput(ctx context.Context, name string, c astiencoder.JobInput) (ctxFormat *avformat.Context, err error) {
	// Open input
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ret := avformat.AvformatOpenInput(&ctxFormat, c.URL, nil, nil); ret < 0 {
			return errors.Wrapf(newAvError(ret), "astilibav: avformat.AvformatOpenInput on input %s with conf %+v failed", name, c)
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

	// Retrieve stream information
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ret := ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
			return errors.Wrapf(newAvError(ret), "astilibav: ctxFormat.AvformatFindStreamInfo on input %s with conf %+v failed", name, c)
		}
		return nil
	}); err != nil {
		return
	}
	return
}

func (o *Opener) OpenOutput(ctx context.Context, name string, c astiencoder.JobOutput) (ctxFormat *avformat.Context, err error) {
	// Alloc format context
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ret := avformat.AvformatAllocOutputContext2(&ctxFormat, nil, "", c.URL); ret < 0 {
			return errors.Wrapf(newAvError(ret), "astilibav: avformat.AvformatAllocOutputContext2 on output %s with conf %+v failed", name, c)
		}
		return nil
	}); err != nil {
		return
	}

	// Make sure the format ctx is properly closed
	o.c.AddCloseFunc(func() error {
		ctxFormat.AvformatFreeContext()
		return nil
	})

	// This is a file
	if ctxFormat.Flags()&avformat.AVFMT_NOFILE == 0 {
		// Open
		var ctxAvIO *avformat.AvIOContext
		if err = astiencoder.CtxFunc(ctx, func() error {
			if ret := avformat.AvIOOpen(&ctxAvIO, c.URL, avformat.AVIO_FLAG_WRITE); ret < 0 {
				return errors.Wrapf(newAvError(ret), "astilibav: avformat.AvIOOpen on output %s with conf %+v failed", name, c)
			}
			return nil
		}); err != nil {
			return
		}

		// Set pb
		if err = astiencoder.CtxFunc(ctx, func() error {
			ctxFormat.SetPb(ctxAvIO)
			return nil
		}); err != nil {
			return
		}

		// Make sure the avio ctx is properly closed
		o.c.AddCloseFunc(func() error {
			if ret := avformat.AvIOClosep(&ctxAvIO); ret < 0 {
				return errors.Wrapf(newAvError(ret), "astilibav: avformat.AvIOClosep on output %s with conf %+v failed", name, c)
			}
			return nil
		})
	}
	return
}
