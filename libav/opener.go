package astilibav

import (
	"context"
	"fmt"
	"github.com/asticode/go-astilog"
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/pkg/errors"
	"github.com/selfmodify/goav/avformat"
)

// Opener represents an object capable of opening a url
type Opener struct {
	c  *astiencoder.Closer
	e  astiencoder.EmitEventFunc
	fs []HandleOpenResultFunc
	m  *sync.Mutex
	t  astiencoder.CreateTaskFunc
}

// NewOpener creates a new Opener
func NewOpener(c *astiencoder.Closer, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) *Opener {
	return &Opener{
		c: c,
		e: e,
		m: &sync.Mutex{},
		t: t,
	}
}

// HandleOpenResultFunc represents an object capable of handling an open result
type HandleOpenResultFunc func(ctx context.Context, ctxFormat *avformat.Context) error

// AddHandleResultFunc adds an open result func
func (o *Opener) AddHandleResultFunc(f HandleOpenResultFunc) {
	o.m.Lock()
	defer o.m.Unlock()
	o.fs = append(o.fs, f)
}

// Open opens a url
func (o *Opener) Open(ctx context.Context, url string) (err error) {
	// Open input
	var ctxFormat *avformat.Context
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ret := avformat.AvformatOpenInput(&ctxFormat, url, nil, nil); ret < 0 {
			return fmt.Errorf("astilibav: avformat.AvformatOpenInput on %s failed with ret = %d", url, ret)
		}
		return nil
	}); err != nil {
		return
	}

	// Make sure the format ctx is properly closed
	o.c.AddCloseFunc(func() error {
		// TODO For now it panics
		astilog.Debugf("astilibav: ctxFormat.AvformatCloseInput() panics :(")
		//ctxFormat.AvformatCloseInput()
		return nil
	})

	// Get funcs
	o.m.Lock()
	fs := append([]HandleOpenResultFunc{}, o.fs...)
	o.m.Unlock()

	// Loop through funcs
	for idx, f := range fs {
		t := o.t()
		go func(idx int, f HandleOpenResultFunc) {
			defer t.Done()
			if err := f(ctx, ctxFormat); err != nil {
				o.e(astiencoder.EventError(errors.Wrapf(err, "astilibav: open handler func #%d failed", idx)))
			}
		}(idx, f)
	}
	return
}
