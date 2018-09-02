package astilibav

import (
	"context"
	"fmt"

	"github.com/asticode/go-astiencoder"
	"github.com/selfmodify/goav/avformat"
)

// Job represents a job
type Job struct {
	URL string `json:"url"`
}

// HandleJob implements the astiencoder.JobHandler interface
func HandleJob(ctx context.Context, job interface{}, e astiencoder.EventEmitter) (err error) {
	// Parse job
	j, ok := job.(Job)
	if !ok {
		err = fmt.Errorf("astilibav: job is not a Job but a %T", job)
		return
	}

	// Open input
	var ctxFormat *avformat.Context
	if err = astiencoder.CtxFunc(ctx, func() error {
		if avformat.AvformatOpenInput(&ctxFormat, j.URL, nil, nil) != 0 {
			return fmt.Errorf("astiencoder: avformat.AvformatOpenInput on %s failed", j.URL)
		}
		return nil
	}); err != nil {
		return
	}
	// For now it panics
	// defer ctxFormat.AvformatCloseInput()

	// Retrieve stream information
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ctxFormat.AvformatFindStreamInfo(nil) < 0 {
			return fmt.Errorf("astiencoder: ctxFormat.AvformatFindStreamInfo on %s failed", j.URL)
		}
		return nil
	}); err != nil {
		return
	}

	// Dump information about file onto standard error
	ctxFormat.AvDumpFormat(0, j.URL, 0)
	return
}
