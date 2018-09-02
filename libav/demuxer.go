package astilibav

import (
	"context"

	"github.com/asticode/go-astitools/worker"
)

// Demuxer represents a demuxer
type Demuxer struct{}

// NewDemuxer creates a new demuxer
func NewDemuxer() *Demuxer {
	return &Demuxer{}
}

// HandleOpenerOutput implements the OpenerOutputHandler interface
func (d *Demuxer) HandleOpenerOutput(ctx context.Context, t *astiworker.Task, o OpenerOutput) (err error) {
	// Dump information about file onto standard error
	o.CtxFormat.AvDumpFormat(0, o.Job.URL, 0)
	return
}
