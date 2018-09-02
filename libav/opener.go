package astilibav

import (
	"context"
	"fmt"
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/selfmodify/goav/avformat"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/go-astilog"
	"github.com/pkg/errors"
)

// Opener represents an object capable of opening stuff
type Opener struct {
	hs []OpenerOutputHandler
	m  *sync.Mutex
}

// NewOpener creates a new Opener
func NewOpener() *Opener {
	return &Opener{
		m: &sync.Mutex{},
	}
}

// OpenerOutput represents an opener output
type OpenerOutput struct {
	CtxFormat *avformat.Context
	Job       Job
}

// OpenerOutputHandler represents an object capable of handling an opener output
type OpenerOutputHandler interface {
	HandleOpenerOutput(ctx context.Context, t *astiworker.Task, o OpenerOutput) error
}

// AddOutputHandler adds an output handler
func (o *Opener) AddOutputHandler(out OpenerOutputHandler) {
	o.m.Lock()
	defer o.m.Unlock()
	o.hs = append(o.hs, out)
}

// Open opens a url
func (o *Opener) Open(ctx context.Context, t *astiworker.Task, j Job) (err error) {
	// Open input
	var out OpenerOutput
	if err = astiencoder.CtxFunc(ctx, func() error {
		if avformat.AvformatOpenInput(&out.CtxFormat, j.URL, nil, nil) != 0 {
			return fmt.Errorf("astilibav: avformat.AvformatOpenInput on %s failed", j.URL)
		}
		return nil
	}); err != nil {
		return
	}
	// TODO For now it panics
	// defer ctxFormat.AvformatCloseInput()

	// Retrieve stream information
	if err = astiencoder.CtxFunc(ctx, func() error {
		if out.CtxFormat.AvformatFindStreamInfo(nil) < 0 {
			return fmt.Errorf("astilibav: ctxFormat.AvformatFindStreamInfo on %s failed", j.URL)
		}
		return nil
	}); err != nil {
		return
	}

	// Handle output
	o.m.Lock()
	for idx, h := range o.hs {
		st := t.NewSubTask()
		go func(idx int, h OpenerOutputHandler) {
			defer st.Done()
			if err := h.HandleOpenerOutput(ctx, t, out); err != nil {
				astilog.Error(errors.Wrapf(err, "astilibav: handling opener output %+v with handler #%d failed", out, idx))
			}
		}(idx, h)
	}
	o.m.Unlock()
	return
}
