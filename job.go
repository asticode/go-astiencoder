package astiencoder

import (
	"context"
	"io"

	"github.com/asticode/go-astitools/worker"
)

// Job represents a job
type Job struct {
	URL string `json:"url"`
}

// CreateTaskFunc is a method that can create a task
type CreateTaskFunc func() *astiworker.Task

// JobHandler represents an object that can handle a job
// Make sure the execution is shut down gracefully when context is cancelled
type JobHandler interface {
	HandleJob(ctx context.Context, j Job, e EmitEventFunc, t CreateTaskFunc) (io.Closer, error)
}
