package astiencoder

import (
	"context"
	"sync"

	"github.com/asticode/go-astitools/worker"
)

// Worker represents a worker that can start and stop
type Worker struct {
	cancel context.CancelFunc
	ctx    context.Context
	oStart *sync.Once
	oStop  *sync.Once
}

// NewWorker creates a new worker
func NewWorker() *Worker {
	return &Worker{
		oStart: &sync.Once{},
		oStop:  &sync.Once{},
	}
}

// Context returns the worker context
func (w *Worker) Context() context.Context {
	return w.ctx
}

// CreateTaskFunc is a method that can create a task
type CreateTaskFunc func() *astiworker.Task

// Start starts the worker
func (w *Worker) Start(ctx context.Context, tc CreateTaskFunc, startFunc func(), execFunc func(t *astiworker.Task)) {
	// Make sure the worker can only be started once
	w.oStart.Do(func() {
		// Check context
		if ctx.Err() != nil {
			return
		}

		// Create task
		t := tc()

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
			execFunc(t)
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
