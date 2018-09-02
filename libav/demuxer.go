package astilibav

import (
	"context"
	"github.com/selfmodify/goav/avformat"

	"github.com/asticode/go-astiencoder"
)

// Demuxer represents a demuxer
type Demuxer struct {
	e astiencoder.EmitEventFunc
	t astiencoder.CreateTaskFunc
}

// NewDemuxer creates a new demuxer
func NewDemuxer(e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) *Demuxer {
	return &Demuxer{
		e: e,
		t: t,
	}
}

// Demux demuxes an input
func (d *Demuxer) Demux(ctx context.Context, ctxFormat *avformat.Context) (err error) {
	// Dump information about file onto standard error
	//o.CtxFormat.AvDumpFormat(0, o.Job.URL, 0)
	return
}
