package astilibav

import (
	"context"
	"fmt"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/worker"
	"github.com/pkg/errors"
)

// Job represents a job
type Job struct {
	URL string `json:"url"`
}

// DefaultJobHandler is the default job handler
var DefaultJobHandler = func(ctx context.Context, t *astiworker.Task, job interface{}, e astiencoder.EventEmitter) (err error) {
	// Parse job
	j, ok := job.(Job)
	if !ok {
		err = fmt.Errorf("astilibav: job is not a Job but a %T", job)
		return
	}

	// Create opener
	o := NewOpener()

	// Create demuxer
	d := NewDemuxer()

	// Connect the demuxer to the opener
	o.AddOutputHandler(d)

	// Open
	if err = o.Open(ctx, t, j); err != nil {
		err = errors.Wrapf(err, "astilibav: opening job %+v failed", j)
		return
	}
	return
}
