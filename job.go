package astiencoder

import (
	"context"
	"io"

	"github.com/asticode/go-astitools/worker"
)

// Job represents a job
type Job struct {
	Inputs    map[string]JobInput   `json:"inputs"`
	Outputs   map[string]JobOutput  `json:"outputs"`
	Processes map[string]JobProcess `json:"processes"`
}

// JobInput represents a job input
type JobInput struct {
	URL string `json:"url"`
}

// JobOutput represents a job output
type JobOutput struct {
	URL string `json:"url"`
}

// Job process types
const (
	JobProcessTypeRemux = "remux"
)

// JobProcess represents a job process
type JobProcess struct {
	Inputs  []JobProcessInput  `json:"inputs"`
	Outputs []JobProcessOutput `json:"outputs"`
	Type    string             `json:"type"`
}

// JobProcessInput  represents a job process input
type JobProcessInput struct {
	Name string   `json:"name"`
	PID  string   `json:"pid"`
}

// JobProcessOutput represents a job process output
type JobProcessOutput struct {
	Name string `json:"name"`
	PID  string `json:"pid"`
}

// CreateTaskFunc is a method that can create a task
type CreateTaskFunc func() *astiworker.Task

// JobHandler represents an object that can handle a job
// Make sure the execution is shut down gracefully when context is cancelled
type JobHandler interface {
	HandleJob(ctx context.Context, j Job, e EmitEventFunc, t CreateTaskFunc) (io.Closer, error)
}
