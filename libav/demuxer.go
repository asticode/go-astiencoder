package astilibav

import (
	"context"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avformat"
)

// Demuxer represents a demuxer
type Demuxer struct {
	c *astiencoder.Closer
	e astiencoder.EmitEventFunc
	t astiencoder.CreateTaskFunc
}

// NewDemuxer creates a new demuxer
func NewDemuxer(c *astiencoder.Closer, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) *Demuxer {
	return &Demuxer{
		c: c,
		e: e,
		t: t,
	}
}

// Demux demuxes an input
func (d *Demuxer) Demux(ctx context.Context, name string, i astiencoder.JobInput, ctxFormat *avformat.Context) (err error) {

	// TODO Loop through streams
	return
}
