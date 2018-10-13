package astilibav

import (
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avformat"
)

// Opener represents an object capable of opening inputs and outputs
type Opener struct {
	c *astiencoder.Closer
}

// NewOpener creates a new Opener
func NewOpener(c *astiencoder.Closer) *Opener {
	return &Opener{
		c: c,
	}
}

// OpenerOptions represents opener options
type OpenerOptions struct {
	Dict        string
	InputFormat *avformat.InputFormat
	URL         string
}

// OpenInput opens an input
func (o *Opener) OpenInput(opts OpenerOptions) (ctxFormat *avformat.Context, err error) {
	// Dict
	var dict *avutil.Dictionary
	if len(opts.Dict) > 0 {
		// Parse dict
		if ret := avutil.AvDictParseString(&dict, opts.Dict, "=", ",", 0); ret < 0 {
			err = errors.Wrapf(newAvError(ret), "astilibav: avutil.AvDictParseString on %s failed", opts.Dict)
			return
		}

		// Make sure the dict is freed
		defer avutil.AvDictFree(&dict)
	}

	// Open input
	if ret := avformat.AvformatOpenInput(&ctxFormat, opts.URL, opts.InputFormat, &dict); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: avformat.AvformatOpenInput on input with options %+v failed", opts)
		return
	}

	// Make sure the format ctx is properly closed
	o.c.Add(func() error {
		avformat.AvformatCloseInput(ctxFormat)
		return nil
	})

	// Retrieve stream information
	if ret := ctxFormat.AvformatFindStreamInfo(nil); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: ctxFormat.AvformatFindStreamInfo on input with options %+v failed", opts)
		return
	}
	return
}

// OpenOutput opens an output
func (o *Opener) OpenOutput(opts OpenerOptions) (ctxFormat *avformat.Context, err error) {
	// Alloc format context
	if ret := avformat.AvformatAllocOutputContext2(&ctxFormat, nil, "", opts.URL); ret < 0 {
		err = errors.Wrapf(newAvError(ret), "astilibav: avformat.AvformatAllocOutputContext2 on output with options %+v failed", opts)
		return
	}

	// Make sure the format ctx is properly closed
	o.c.Add(func() error {
		ctxFormat.AvformatFreeContext()
		return nil
	})

	// This is a file
	if ctxFormat.Flags()&avformat.AVFMT_NOFILE == 0 {
		// Open
		var ctxAvIO *avformat.AvIOContext
		if ret := avformat.AvIOOpen(&ctxAvIO, opts.URL, avformat.AVIO_FLAG_WRITE); ret < 0 {
			err = errors.Wrapf(newAvError(ret), "astilibav: avformat.AvIOOpen on output with options %+v failed", opts)
			return
		}

		// Set pb
		ctxFormat.SetPb(ctxAvIO)

		// Make sure the avio ctx is properly closed
		o.c.Add(func() error {
			if ret := avformat.AvIOClosep(&ctxAvIO); ret < 0 {
				return errors.Wrapf(newAvError(ret), "astilibav: avformat.AvIOClosep on output with options %+v failed", opts)
			}
			return nil
		})
	}
	return
}
