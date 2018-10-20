package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avfilter"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

var countFilterer uint64

// Filterer represents an object capable of applying a filter to frames
type Filterer struct {
	*astiencoder.BaseNode
	bufferSinkCtx    *avfilter.Context
	bufferSrcCtx     *avfilter.Context
	d                *frameDispatcher
	e                *astiencoder.EventEmitter
	g                *avfilter.Graph
	q                *astisync.CtxQueue
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// NewFilterer creates a new filterer
func NewFilterer(bufferSrcCtx, bufferSinkCtx *avfilter.Context, g *avfilter.Graph, e *astiencoder.EventEmitter, c *astiencoder.Closer) (f *Filterer) {
	count := atomic.AddUint64(&countFilterer, uint64(1))
	f = &Filterer{
		BaseNode: astiencoder.NewBaseNode(e, astiencoder.NodeMetadata{
			Description: "Filters",
			Label:       fmt.Sprintf("Filterer #%d", count),
			Name:        fmt.Sprintf("filterer_%d", count),
		}),
		bufferSinkCtx:    bufferSinkCtx,
		bufferSrcCtx:     bufferSrcCtx,
		d:                newFrameDispatcher(e, c),
		e:                e,
		g:                g,
		q:                astisync.NewCtxQueue(),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	f.addStats()
	return
}

// FiltererOptions represents filterer options
type FiltererOptions struct {
	Content string
	Input   Context
}

// NewFiltererFromOptions creates a new filterer based on options
func NewFiltererFromOptions(o FiltererOptions, e *astiencoder.EventEmitter, c *astiencoder.Closer) (_ *Filterer, err error) {
	// Create graph
	g := avfilter.AvfilterGraphAlloc()
	c.Add(func() error {
		g.AvfilterGraphFree()
		return nil
	})

	// Create buffers
	bufferSrc := avfilter.AvfilterGetByName("buffer")
	bufferSink := avfilter.AvfilterGetByName("buffersink")

	// Create filter in args
	var args string
	switch o.Input.CodecType {
	case avcodec.AVMEDIA_TYPE_VIDEO:
		args = fmt.Sprintf("video_size=%dx%d:pix_fmt=%d:time_base=%d/%d:pixel_aspect=%d/%d", o.Input.Width, o.Input.Height, o.Input.PixelFormat, o.Input.TimeBase.Num(), o.Input.TimeBase.Den(), o.Input.SampleAspectRatio.Num(), o.Input.SampleAspectRatio.Den())
	default:
		err = fmt.Errorf("astilibav: codec type %v is not handled by filterer", o.Input.CodecType)
		return
	}

	// Create filter in
	var bufferSrcCtx *avfilter.Context
	if ret := avfilter.AvfilterGraphCreateFilter(&bufferSrcCtx, bufferSrc, "in", args, nil, g); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: avfilter.AvfilterGraphCreateFilter on args %s failed", args)
		return
	}

	// Create filter sink
	var bufferSinkCtx *avfilter.Context
	if ret := avfilter.AvfilterGraphCreateFilter(&bufferSinkCtx, bufferSink, "out", "", nil, g); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: avfilter.AvfilterGraphCreateFilter on empty args failed")
		return
	}

	// Create outputs
	outputs := avfilter.AvfilterInoutAlloc()
	defer avfilter.AvfilterInoutFree(&outputs)
	outputs.SetName("in")
	outputs.SetFilterCtx(bufferSrcCtx)
	outputs.SetPadIdx(0)
	outputs.SetNext(nil)

	// Create inputs
	inputs := avfilter.AvfilterInoutAlloc()
	defer avfilter.AvfilterInoutFree(&inputs)
	inputs.SetName("out")
	inputs.SetFilterCtx(bufferSinkCtx)
	inputs.SetPadIdx(0)
	inputs.SetNext(nil)

	// Parse content
	if ret := g.AvfilterGraphParsePtr(o.Content, &inputs, &outputs, nil); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: g.AvfilterGraphParsePtr on content %s failed", o.Content)
		return
	}

	// Configure
	if ret := g.AvfilterGraphConfig(nil); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: g.AvfilterGraphConfig failed")
		return
	}

	// Create filterer
	return NewFilterer(bufferSrcCtx, bufferSinkCtx, g, e, c), nil
}

func (f *Filterer) addStats() {
	// Add incoming rate
	f.Stater().AddStat(astistat.StatMetadata{
		Description: "Number of frames coming in the filter per second",
		Label:       "Incoming rate",
		Unit:        "fps",
	}, f.statIncomingRate)

	// Add work ratio
	f.Stater().AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, f.statWorkRatio)

	// Add dispatcher stats
	f.d.addStats(f.Stater())

	// Add queue stats
	f.q.AddStats(f.Stater())
}

// Connect implements the FrameHandlerConnector interface
func (f *Filterer) Connect(h FrameHandler) {
	// Append handler
	f.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(f, h.(astiencoder.Node))
}

// Start starts the filterer
func (f *Filterer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	f.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go f.q.HandleCtx(f.Context())

		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer f.d.wait()

		// Make sure to stop the queue properly
		defer f.q.Stop()

		// Start queue
		f.q.Start(func(dp interface{}) {
			// Handle pause
			defer f.HandlePause()

			// Assert payload
			p := dp.(*FrameHandlerPayload)

			// Increment incoming rate
			f.statIncomingRate.Add(1)

			// Push frame in graph
			f.statWorkRatio.Add(true)
			if ret := f.g.AvBuffersrcAddFrameFlags(f.bufferSrcCtx, p.Frame, avfilter.AV_BUFFERSRC_FLAG_KEEP_REF); ret < 0 {
				f.statWorkRatio.Done(true)
				emitAvError(f.e, ret, "f.g.AvBuffersrcAddFrameFlags failed")
				return
			}
			f.statWorkRatio.Done(true)

			// Loop
			for {
				// Pull filtered frame
				if stop := f.pullFilteredFrame(p.Prev); stop {
					return
				}
			}
		})
	})
}

func (f *Filterer) pullFilteredFrame(prev Descriptor) (stop bool) {
	// Get frame
	fm := f.d.getFrame()
	defer f.d.putFrame(fm)

	// Pull filtered frame from graph
	f.statWorkRatio.Add(true)
	if ret := f.g.AvBuffersinkGetFrame(f.bufferSinkCtx, fm); ret < 0 {
		f.statWorkRatio.Done(true)
		if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
			emitAvError(f.e, ret, "f.g.AvBuffersinkGetFrame failed")
		}
		stop = true
		return
	}
	f.statWorkRatio.Done(true)

	// Dispatch frame
	f.d.dispatch(fm, newFiltererPrev(f.bufferSinkCtx, prev))
	return
}

// HandleFrame implements the FrameHandler interface
func (f *Filterer) HandleFrame(p *FrameHandlerPayload) {
	f.q.Send(p)
}

type filtererPrev struct {
	bufferSinkCtx *avfilter.Context
	prev          Descriptor
}

func newFiltererPrev(bufferSinkCtx *avfilter.Context, prev Descriptor) *filtererPrev {
	return &filtererPrev{
		bufferSinkCtx: bufferSinkCtx,
		prev:          prev,
	}
}

// TimeBase implements the Descriptor interface
func (p *filtererPrev) TimeBase() avutil.Rational {
	if p.bufferSinkCtx.NbInputs() == 0 {
		return p.prev.TimeBase()
	}
	return p.bufferSinkCtx.Inputs()[0].TimeBase()
}
