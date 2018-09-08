package astilibav

import (
	"context"
	"sync"

	"github.com/asticode/go-astiencoder"
)

type worker struct {
	cancel context.CancelFunc
	ctx    context.Context
	o      *sync.Once
	t      astiencoder.CreateTaskFunc
}

func newWorker(t astiencoder.CreateTaskFunc) *worker {
	return &worker{
		o:         &sync.Once{},
		t:         t,
	}
}

func (w *worker) start(ctx context.Context, startFunc func(), execFunc func()) {
	// Make sure the worker can only be started once
	w.o.Do(func() {
		// Check context
		if ctx.Err() != nil {
			return
		}

		// Create task
		t := w.t()

		// Reset context
		w.ctx, w.cancel = context.WithCancel(ctx)

		// Start func
		if startFunc != nil {
			startFunc()
		}

		// Execute the rest in a goroutine
		go func() {
			// Task is done
			defer t.Done()

			// Make sure the worker is stopped properly
			defer w.stop()

			// Exec func
			execFunc()
		}()
	})
}

func (w *worker) stop() {
	// Cancel context
	if w.cancel != nil {
		w.cancel()
	}

	// Reset once
	w.o = &sync.Once{}
}
