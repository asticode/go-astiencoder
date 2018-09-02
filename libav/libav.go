package astilibav

import (
	"context"

	"github.com/asticode/go-astiencoder"
	"github.com/pkg/errors"
	"github.com/selfmodify/goav/avcodec"
	"github.com/selfmodify/goav/avformat"
)

func init() {
	avformat.AvRegisterAll()
	avcodec.AvcodecRegisterAll()
}

// DefaultJobHandler is the default libav job handler
var DefaultJobHandler = func(ctx context.Context, j astiencoder.Job, e astiencoder.EventEmitter, t astiencoder.TaskCreator) (err error) {
	// Create opener
	o := NewOpener(e, t)

	// Create demuxer
	d := NewDemuxer(e, t)

	// Connect the demuxer to the opener
	o.AddHandler(d.Demux)

	// Open
	if err = o.Open(ctx, j.URL); err != nil {
		err = errors.Wrapf(err, "astilibav: opening job %+v failed", j)
		return
	}
	return
}
