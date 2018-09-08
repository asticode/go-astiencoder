package main

import (
	"context"
	"fmt"
	"io"

	"github.com/asticode/goav/avcodec"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/go-astilog"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

type handler struct{}

func newHandler() *handler {
	return &handler{}
}

type openedInput struct {
	ctxFormat *avformat.Context
	d         *astilibav.Demuxer
	i         astiencoder.JobInput
}

type openedOutput struct {
	ctxFormat *avformat.Context
	m         *astilibav.Muxer
	o         astiencoder.JobOutput
}

// HandleJob implements the astiencoder.JobHandler interface
func (h *handler) HandleJob(ctx context.Context, j astiencoder.Job, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) (cls io.Closer, err error) {
	// Create closer
	c := astiencoder.NewCloser()
	cls = c

	// Create opener
	o := astilibav.NewOpener(c)

	// No inputs
	if len(j.Inputs) == 0 {
		err = errors.New("main: no inputs provided")
		return
	}

	// Open inputs
	var is map[string]openedInput
	if is, err = h.openInputs(ctx, j, o, e, t); err != nil {
		err = errors.Wrap(err, "main: opening inputs failed")
		return
	}

	// No outputs
	if len(j.Outputs) == 0 {
		err = errors.New("main: no outputs provided")
		return
	}

	// Open outputs
	var os map[string]openedOutput
	if os, err = h.openOutputs(ctx, j, o, t); err != nil {
		err = errors.Wrap(err, "main: opening outputs failed")
		return
	}

	// No processes
	if len(j.Processes) == 0 {
		err = errors.New("main: no processes provided")
		return
	}

	// Handle processes
	if err = h.handleProcesses(ctx, j, is, os); err != nil {
		err = errors.Wrap(err, "main: handling processes failed")
		return
	}
	return c, nil
}

func (h *handler) openInputs(ctx context.Context, j astiencoder.Job, o *astilibav.Opener, e astiencoder.EmitEventFunc, t astiencoder.CreateTaskFunc) (is map[string]openedInput, err error) {
	// Loop through inputs
	is = make(map[string]openedInput)
	for n, i := range j.Inputs {
		// Open
		var ctxFormat *avformat.Context
		if ctxFormat, err = o.OpenInput(ctx, n, i); err != nil {
			err = errors.Wrapf(err, "main: opening input %s with conf %+v failed", n, i)
			return
		}

		// Index
		is[n] = openedInput{
			ctxFormat: ctxFormat,
			d:         astilibav.NewDemuxer(ctxFormat, e, t),
			i:         i,
		}
	}
	return
}

func (h *handler) openOutputs(ctx context.Context, j astiencoder.Job, o *astilibav.Opener, t astiencoder.CreateTaskFunc) (os map[string]openedOutput, err error) {
	// Loop through outputs
	os = make(map[string]openedOutput)
	for n, out := range j.Outputs {
		// Open
		var ctxFormat *avformat.Context
		if ctxFormat, err = o.OpenOutput(ctx, n, out); err != nil {
			err = errors.Wrapf(err, "main: opening output %s with conf %+v failed", n, out)
			return
		}

		// Index
		os[n] = openedOutput{
			ctxFormat: ctxFormat,
			m:         astilibav.NewMuxer(ctxFormat, t),
			o:         out,
		}
	}
	return
}

func (h *handler) handleProcesses(ctx context.Context, j astiencoder.Job, is map[string]openedInput, os map[string]openedOutput) (err error) {
	// Loop through processes
	var ds = make(map[*astilibav.Demuxer]bool)
	var ms = make(map[*astilibav.Muxer]bool)
	for n, p := range j.Processes {
		// Handle process
		if err = h.handleProcess(ctx, n, p, is, os, ds, ms); err != nil {
			err = errors.Wrapf(err, "main: handling process %s with conf %+v failed", n, p)
			return
		}
	}

	// Start muxers
	for m := range ms {
		m.Start(ctx)
	}

	// Start demuxers
	for d := range ds {
		d.Start(ctx)
	}
	return
}

func (h *handler) handleProcess(ctx context.Context, name string, p astiencoder.JobProcess, ois map[string]openedInput, oos map[string]openedOutput, ds map[*astilibav.Demuxer]bool, ms map[*astilibav.Muxer]bool) (err error) {
	// No inputs
	if len(p.Inputs) == 0 {
		err = errors.New("main: no process inputs provided")
		return
	}

	// Loop through inputs
	var is []openedInput
	for _, pi := range p.Inputs {
		// Retrieve opened input
		i, ok := ois[pi.Name]
		if !ok {
			err = fmt.Errorf("main: opened input %s not found", pi.Name)
			return
		}

		// Append input
		is = append(is, i)
	}

	// No outputs
	if len(p.Outputs) == 0 {
		err = errors.New("main: no process outputs provided")
		return
	}

	// Loop through outputs
	var os []openedOutput
	for _, po := range p.Outputs {
		// Retrieve opened output
		o, ok := oos[po.Name]
		if !ok {
			err = fmt.Errorf("main: opened output %s not found", po.Name)
			return
		}

		// Append output
		os = append(os, o)
	}

	// Switch on process type
	switch p.Type {
	case astiencoder.JobProcessTypeRemux:
		err = h.handleProcessRemux(ctx, is, os, ds, ms)
	default:
		astilog.Warnf("main: unhandled job process type %s", p.Type)
	}
	return
}

func (h *handler) handleProcessRemux(ctx context.Context, is []openedInput, os []openedOutput, ds map[*astilibav.Demuxer]bool, ms map[*astilibav.Muxer]bool) (err error) {
	// Loop through inputs
	for _, i := range is {
		// Loop through streams
		for _, s := range i.ctxFormat.Streams() {
			// Only process some media types
			if s.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_AUDIO &&
				s.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_SUBTITLE &&
				s.CodecParameters().CodecType() != avcodec.AVMEDIA_TYPE_VIDEO {
				continue
			}

			// Handle demux pkt
			i.d.OnPkt(s.Index(), func(pkt *avcodec.Packet) {
				// TODO Copy packet

				// Send packet to muxers
				for _, o := range os {
					o.m.SendPkt(pkt)
				}
			})

			// Index demuxer
			ds[i.d] = true

			// Loop through outputs
			for _, o := range os {
				// Check if muxer is already indexed
				if _, ok := ms[o.m]; !ok {
					// Index muxer
					ms[o.m] = true

					// Handle demux eof
					i.d.OnEOF(func() {
						o.m.Stop()
					})
				}
			}
		}
	}
	return
}

// HandleCmd implements the astiencoder.CmdHandler interface
func (h *handler) HandleCmd(c astiencoder.Cmd) {
	// TODO Handle cmd
	astilog.Warnf("main: TODO: handle cmd %+v", c)
}
