package astiencoder

import (
	"context"
	"github.com/asticode/go-astitools/worker"
)

// Job represents a job
type Job struct {
	URL string `json:"url"`
}

// TaskCreator is a method that can create a task
type TaskCreator func() *astiworker.Task

// JobHandler handles a job
// Make sure the execution is shut down gracefully when context is cancelled
type JobHandler func(ctx context.Context, j Job, e EventEmitter, t TaskCreator) error
