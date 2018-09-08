package astiencoder

import (
	"context"
	"sync"
)

// Worker represents a worker that can start and stop
type Worker struct {
	cancel context.CancelFunc
	ctx    context.Context
	oStart *sync.Once
	oStop  *sync.Once
	t      CreateTaskFunc
}

// NewWorker creates a new worker
func NewWorker(t CreateTaskFunc) *Worker {
	return &Worker{
		oStart: &sync.Once{},
		oStop:  &sync.Once{},
		t:      t,
	}
}

// Ctx returns the worker's ctx
func (w *Worker) Ctx() context.Context {
	return w.ctx
}

// Start starts the worker
func (w *Worker) Start(ctx context.Context, startFunc func(), execFunc func()) {
	// Make sure the worker can only be started once
	w.oStart.Do(func() {
		// Check context
		if ctx.Err() != nil {
			return
		}

		// Create task
		t := w.t()

		// Reset context
		w.ctx, w.cancel = context.WithCancel(ctx)

		// Reset once
		w.oStop = &sync.Once{}

		// Start func
		if startFunc != nil {
			startFunc()
		}

		// Execute the rest in a goroutine
		go func() {
			// Task is done
			defer t.Done()

			// Make sure the worker is stopped properly
			defer w.Stop()

			// Exec func
			execFunc()
		}()
	})
}

// Stop stops the worker
func (w *Worker) Stop() {
	// Make sure the worker can only be stopped once
	w.oStop.Do(func() {
		// Cancel context
		if w.cancel != nil {
			w.cancel()
		}

		// Reset once
		w.oStart = &sync.Once{}
	})
}
