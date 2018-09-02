package astilibav

import (
	"context"
	"fmt"
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/pkg/errors"
	"github.com/selfmodify/goav/avformat"
)

// Opener represents an object capable of opening a url
type Opener struct {
	e  astiencoder.EmitEventFunc
	hs []OpenerHandler
	m  *sync.Mutex
	t  astiencoder.CreateTaskFunc
}

// NewOpener creates a new Opener
func NewOpener(e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) *Opener {
	return &Opener{
		e: e,
		m: &sync.Mutex{},
		t: t,
	}
}

// OpenerHandler represents an object capable of handling an opener result
type OpenerHandler func(ctx context.Context, ctxFormat *avformat.Context) error

// AddHandler adds a handler
func (o *Opener) AddHandler(out OpenerHandler) {
	o.m.Lock()
	defer o.m.Unlock()
	o.hs = append(o.hs, out)
}

// Open opens a url
func (o *Opener) Open(ctx context.Context, url string) (err error) {
	// Open input
	var ctxFormat *avformat.Context
	if err = astiencoder.CtxFunc(ctx, func() error {
		if avformat.AvformatOpenInput(&ctxFormat, url, nil, nil) != 0 {
			return fmt.Errorf("astilibav: avformat.AvformatOpenInput on %s failed", url)
		}
		return nil
	}); err != nil {
		return
	}
	// TODO For now it panics
	// defer ctxFormat.AvformatCloseInput()

	// Retrieve stream information
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ctxFormat.AvformatFindStreamInfo(nil) < 0 {
			return fmt.Errorf("astilibav: ctxFormat.AvformatFindStreamInfo on %s failed", url)
		}
		return nil
	}); err != nil {
		return
	}

	// Handle output
	o.m.Lock()
	for idx, h := range o.hs {
		t := o.t()
		go func(idx int, h OpenerHandler) {
			defer t.Done()
			if err := h(ctx, ctxFormat); err != nil {
				o.e(astiencoder.EventError(errors.Wrapf(err, "astilibav: opener handler #%d failed", idx)))
			}
		}(idx, h)
	}
	o.m.Unlock()
	return
}
