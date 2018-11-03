package main

import (
	"fmt"
	"strings"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

func addWorkflow(name string, j Job, e *encoder) (w *astiencoder.Workflow, err error) {
	// Create closer
	c := astiencoder.NewCloser()

	// Create workflow
	w = astiencoder.NewWorkflow(e.w.Context(), name, e.ee, e.w.NewTask, c)

	// Build workflow
	b := newBuilder()
	if err = b.buildWorkflow(j, w, e.ee, c); err != nil {
		err = errors.Wrap(err, "main: building workflow failed")
		return
	}

	// Index nodes
	w.IndexNodes()

	// Add workflow to pool
	e.wp.AddWorkflow(w)
	return
}

type builder struct{}

func newBuilder() *builder {
	return &builder{}
}

type openedInput struct {
	c JobInput
	d *astilibav.Demuxer
}

type openedOutput struct {
	c JobOutput
	m *astilibav.Muxer
}

type buildData struct {
	c        *astiencoder.Closer
	decoders map[*astilibav.Demuxer]map[*avformat.Stream]*astilibav.Decoder
	ee       *astiencoder.EventEmitter
	inputs   map[string]openedInput
	outputs  map[string]openedOutput
	w        *astiencoder.Workflow
}

func newBuildData(w *astiencoder.Workflow, ee *astiencoder.EventEmitter, c *astiencoder.Closer) *buildData {
	return &buildData{
		c:        c,
		ee:       ee,
		decoders: make(map[*astilibav.Demuxer]map[*avformat.Stream]*astilibav.Decoder),
		w:        w,
	}
}

func (b *builder) buildWorkflow(j Job, w *astiencoder.Workflow, ee *astiencoder.EventEmitter, c *astiencoder.Closer) (err error) {
	// Create build data
	bd := newBuildData(w, ee, c)

	// No inputs
	if len(j.Inputs) == 0 {
		err = errors.New("main: no inputs provided")
		return
	}

	// Open inputs
	if bd.inputs, err = b.openInputs(j, bd); err != nil {
		err = errors.Wrap(err, "main: opening inputs failed")
		return
	}

	// No outputs
	if len(j.Outputs) == 0 {
		err = errors.New("main: no outputs provided")
		return
	}

	// Open outputs
	if bd.outputs, err = b.openOutputs(j, bd); err != nil {
		err = errors.Wrap(err, "main: opening outputs failed")
		return
	}

	// No operations
	if len(j.Operations) == 0 {
		err = errors.New("main: no operations provided")
		return
	}

	// Loop through operations
	for n, o := range j.Operations {
		// Add operation to workflow
		if err = b.addOperationToWorkflow(n, o, bd); err != nil {
			err = errors.Wrapf(err, "main: adding operation %s with conf %+v to workflow failed", n, o)
			return
		}
	}
	return
}

func (b *builder) openInputs(j Job, bd *buildData) (is map[string]openedInput, err error) {
	// Loop through inputs
	is = make(map[string]openedInput)
	for n, cfg := range j.Inputs {
		// Create demuxer
		var d *astilibav.Demuxer
		if d, err = astilibav.NewDemuxer(astilibav.DemuxerOptions{
			Dict:        cfg.Dict,
			EmulateRate: cfg.EmulateRate,
			URL:         cfg.URL,
		}, bd.ee, bd.c); err != nil {
			err = errors.Wrap(err, "main: creating demuxer failed")
			return
		}

		// Index
		is[n] = openedInput{
			c: cfg,
			d: d,
		}
	}
	return
}

func (b *builder) openOutputs(j Job, bd *buildData) (os map[string]openedOutput, err error) {
	// Loop through outputs
	os = make(map[string]openedOutput)
	for n, cfg := range j.Outputs {
		// Create output
		oo := openedOutput{
			c: cfg,
		}

		// Switch on type
		switch cfg.Type {
		case JobOutputTypePktDump:
			// This is a per-operation and per-input value since we may want to index the path by input name
			// The writer is created afterwards
		default:
			// Create muxer
			if oo.m, err = astilibav.NewMuxer(astilibav.MuxerOptions{URL: cfg.URL}, bd.ee, bd.c); err != nil {
				err = errors.Wrap(err, "main: creating muxer failed")
				return
			}
		}

		// Index
		os[n] = oo
	}
	return
}

type operationInput struct {
	c JobOperationInput
	o openedInput
}

type operationOutput struct {
	c JobOperationOutput
	o openedOutput
}

func (b *builder) addOperationToWorkflow(name string, o JobOperation, bd *buildData) (err error) {
	// Get operation inputs and outputs
	var ois []operationInput
	var oos []operationOutput
	if ois, oos, err = b.operationInputsOutputs(o, bd); err != nil {
		err = errors.Wrapf(err, "main: getting inputs and outputs of operation %s failed", name)
		return
	}

	// Loop through inputs
	for _, i := range ois {
		// Loop through streams
		for _, is := range i.o.d.CtxFormat().Streams() {
			// Only process a specific PID
			if i.c.PID != nil && is.Id() != *i.c.PID {
				continue
			}

			// Only process a specific media type
			if t := avutil.MediaTypeFromString(i.c.MediaType); t > -1 && is.CodecParameters().CodecType() != avcodec.MediaType(t) {
				continue
			}

			// Add demuxer as root node of the workflow
			bd.w.AddChild(i.o.d)

			// In case of copy we only want to connect the demuxer to the muxer
			if o.Codec == JobOperationCodecCopy {
				// Loop through outputs
				for _, o := range oos {
					// Clone stream
					var os *avformat.Stream
					if os, err = astilibav.CloneStream(is, o.o.m.CtxFormat()); err != nil {
						err = errors.Wrapf(err, "main: cloning stream 0x%x(%d) of %s failed", is.Id(), is.Id(), i.c.Name)
						return
					}

					// Create muxer handler
					h := o.o.m.NewPktHandler(os)

					// Connect demuxer to handler
					i.o.d.ConnectForStream(h, is)
				}
				continue
			}

			// Create decoder
			var d *astilibav.Decoder
			if d, err = b.createDecoder(bd, i, is); err != nil {
				err = errors.Wrapf(err, "main: creating decoder for stream 0x%x(%d) of input %s failed", is.Id(), is.Id(), i.c.Name)
				return
			}

			// Create input ctx
			inCtx := astilibav.NewContextFromStream(is)

			// Create output ctx
			outCtx := b.operationOutputCtx(o, inCtx, oos)

			// Create filterer
			var f *astilibav.Filterer
			if f, err = b.createFilterer(bd, inCtx, outCtx, d); err != nil {
				err = errors.Wrapf(err, "main: creating filterer for stream 0x%x(%d) of input %s failed", is.Id(), is.Id(), i.c.Name)
				return
			}

			// Create encoder
			var e *astilibav.Encoder
			if e, err = astilibav.NewEncoderFromContext(outCtx, bd.ee, bd.c); err != nil {
				err = errors.Wrapf(err, "main: creating encoder for stream 0x%x(%d) of input %s failed", is.Id(), is.Id(), i.c.Name)
				return
			}

			// Connect demuxer or filterer to encoder
			if f != nil {
				d.Connect(f)
				f.Connect(e)
			} else {
				d.Connect(e)
			}

			// Loop through outputs
			for _, o := range oos {
				// Switch on type
				var h astilibav.PktHandler
				switch o.o.c.Type {
				case JobOutputTypePktDump:
					// Create pkt dumper
					if h, err = astilibav.NewPktDumper(astilibav.PktDumperOptions{
						Data:    map[string]interface{}{"input": i.c.Name},
						Handler: astilibav.PktDumpFile,
						Pattern: o.o.c.URL,
					}, bd.ee); err != nil {
						err = errors.Wrapf(err, "main: creating pkt dumper for output %s with conf %+v failed", o.c.Name, o.c)
						return
					}
				default:
					// Add stream
					var os *avformat.Stream
					if os, err = e.AddStream(o.o.m.CtxFormat()); err != nil {
						err = errors.Wrapf(err, "main: adding stream for stream 0x%x(%d) of %s and output %s failed", is.Id(), is.Id(), i.c.Name, o.c.Name)
						return
					}

					// Create muxer handler
					h = o.o.m.NewPktHandler(os)
				}

				// Connect encoder to handler
				e.Connect(h)
			}
		}
	}
	return
}

func (b *builder) operationInputsOutputs(o JobOperation, bd *buildData) (is []operationInput, os []operationOutput, err error) {
	// No inputs
	if len(o.Inputs) == 0 {
		err = errors.New("main: no operation inputs provided")
		return
	}

	// Loop through inputs
	for _, pi := range o.Inputs {
		// Retrieve opened input
		i, ok := bd.inputs[pi.Name]
		if !ok {
			err = fmt.Errorf("main: opened input %s not found", pi.Name)
			return
		}

		// Append input
		is = append(is, operationInput{
			c: pi,
			o: i,
		})
	}

	// No outputs
	if len(o.Outputs) == 0 {
		err = errors.New("main: no operation outputs provided")
		return
	}

	// Loop through outputs
	for _, po := range o.Outputs {
		// Retrieve opened output
		o, ok := bd.outputs[po.Name]
		if !ok {
			err = fmt.Errorf("main: opened output %s not found", po.Name)
			return
		}

		// Append output
		os = append(os, operationOutput{
			c: po,
			o: o,
		})
	}
	return
}

func (b *builder) operationOutputCtx(o JobOperation, inCtx astilibav.Context, oos []operationOutput) (outCtx astilibav.Context) {
	// Default output ctx is input ctx
	outCtx = inCtx

	// Set codec name
	outCtx.CodecName = o.Codec

	// Set frame rate
	if o.FrameRate != nil {
		outCtx.FrameRate = avutil.NewRational(o.FrameRate.Num, o.FrameRate.Den)
	}

	// Set time base
	if o.TimeBase != nil {
		outCtx.TimeBase = avutil.NewRational(o.TimeBase.Num, o.TimeBase.Den)
	} else {
		outCtx.TimeBase = avutil.NewRational(outCtx.FrameRate.Den(), outCtx.FrameRate.Num())
	}

	// Set pixel format
	if len(o.PixelFormat) > 0 {
		outCtx.PixelFormat = avutil.PixelFormatFromString(o.PixelFormat)
	} else if o.Codec == "mjpeg" {
		outCtx.PixelFormat = avutil.AV_PIX_FMT_YUVJ420P
	}

	// Set height
	if o.Height != nil {
		outCtx.Height = *o.Height
	}

	// Set width
	if o.Width != nil {
		outCtx.Width = *o.Width
	}

	// Set bit rate
	if o.BitRate != nil {
		outCtx.BitRate = *o.BitRate
	}

	// Set gop size
	if o.GopSize != nil {
		outCtx.GopSize = *o.GopSize
	}

	// Set thread count
	outCtx.ThreadCount = o.ThreadCount

	// Set dict
	outCtx.Dict = o.Dict

	// TODO Add audio options

	// Set global header
	if oos[0].o.m != nil {
		outCtx.GlobalHeader = oos[0].o.m.CtxFormat().Oformat().Flags()&avformat.AVFMT_GLOBALHEADER > 0
	}
	return
}

func (b *builder) createDecoder(bd *buildData, i operationInput, is *avformat.Stream) (d *astilibav.Decoder, err error) {
	// Get decoder
	var okD, okS bool
	if _, okD = bd.decoders[i.o.d]; okD {
		d, okS = bd.decoders[i.o.d][is]
	} else {
		bd.decoders[i.o.d] = make(map[*avformat.Stream]*astilibav.Decoder)
	}

	// Decoder doesn't exist
	if !okD || !okS {
		// Create decoder
		if d, err = astilibav.NewDecoderFromCodecParams(is.CodecParameters(), bd.ee, bd.c); err != nil {
			err = errors.Wrapf(err, "main: creating decoder for stream 0x%x(%d) of %s failed", is.Id(), is.Id(), i.c.Name)
			return
		}

		// Connect demuxer
		i.o.d.ConnectForStream(d, is)

		// Index decoder
		bd.decoders[i.o.d][is] = d
	}
	return
}

func (b *builder) createFilterer(bd *buildData, inCtx, outCtx astilibav.Context, n astiencoder.Node) (f *astilibav.Filterer, err error) {
	// Create filters
	var filters []string

	// Switch on media type
	switch inCtx.CodecType {
	case avutil.AVMEDIA_TYPE_VIDEO:
		// Frame rate
		// TODO Use select if inFramerate > outFramerate
		if inCtx.FrameRate.Den() > 0 && outCtx.FrameRate.Den() > 0 && inCtx.FrameRate.Num()/inCtx.FrameRate.Den() != outCtx.FrameRate.Num()/outCtx.FrameRate.Den() {
			filters = append(filters, fmt.Sprintf("minterpolate='fps=%d/%d'", outCtx.FrameRate.Num(), outCtx.FrameRate.Den()))
		}

		// Scale
		if inCtx.Height != outCtx.Height || inCtx.Width != outCtx.Width {
			filters = append(filters, fmt.Sprintf("scale='w=%d:h=%d'", outCtx.Width, outCtx.Height))
		}
	}

	// There are filters
	if len(filters) > 0 {
		// Create filterer options
		fo := astilibav.FiltererOptions{
			Content: strings.Join(filters, ","),
			Inputs: map[string]astilibav.FiltererInput{
				"in": {
					Context: inCtx,
					Node:    n,
				},
			},
		}

		// Create filterer
		if f, err = astilibav.NewFiltererFromOptions(fo, bd.ee, bd.c); err != nil {
			err = errors.Wrapf(err, "main: creating filterer with filters %+v failed", filters)
			return
		}
	}
	return
}
