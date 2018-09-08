package astilibav

import (
	"context"
	"fmt"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avformat"
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
	// Retrieve stream information
	if err = astiencoder.CtxFunc(ctx, func() error {
		if ret := ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
			return fmt.Errorf("astilibav: ctxFormat.AvformatFindStreamInfo on %s failed with ret = %d", ctxFormat.Filename(), ret)
		}
		return nil
	}); err != nil {
		return
	}

	// TODO Loop through streams
	return
}
