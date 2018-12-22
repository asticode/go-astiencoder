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
	bufferSrcCtxs    map[astiencoder.Node]*avfilter.Context
	d                *frameDispatcher
	e                astiencoder.EventEmitter
	g                *avfilter.Graph
	q                *astisync.CtxQueue
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// NewFilterer creates a new filterer
func NewFilterer(bufferSrcCtxs map[astiencoder.Node]*avfilter.Context, bufferSinkCtx *avfilter.Context, g *avfilter.Graph, e astiencoder.EventEmitter, c astiencoder.CloseFuncAdder) (f *Filterer) {
	count := atomic.AddUint64(&countFilterer, uint64(1))
	f = &Filterer{
		bufferSinkCtx:    bufferSinkCtx,
		bufferSrcCtxs:    bufferSrcCtxs,
		e:                e,
		g:                g,
		q:                astisync.NewCtxQueue(),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	f.BaseNode = astiencoder.NewBaseNode(astiencoder.NewEventGeneratorNode(f), e, astiencoder.NodeMetadata{
		Description: "Filters",
		Label:       fmt.Sprintf("Filterer #%d", count),
		Name:        fmt.Sprintf("filterer_%d", count),
	})
	f.d = newFrameDispatcher(f, e, c)
	f.addStats()
	return
}

// FiltererOptions represents filterer options
type FiltererOptions struct {
	Content string
	Inputs  map[string]FiltererInput
}

// FiltererInput represents a filterer input
type FiltererInput struct {
	Context Context
	Node    astiencoder.Node
}

// NewFiltererFromOptions creates a new filterer based on options
func NewFiltererFromOptions(o FiltererOptions, e astiencoder.EventEmitter, c astiencoder.CloseFuncAdder) (_ *Filterer, err error) {
	// Create graph
	g := avfilter.AvfilterGraphAlloc()
	c.Add(func() error {
		g.AvfilterGraphFree()
		return nil
	})

	// No inputs
	if len(o.Inputs) == 0 {
		err = errors.New("astilibav: no inputs in filterer options")
		return
	}

	// Get codec type
	codecType := avcodec.MediaType(avcodec.AVMEDIA_TYPE_UNKNOWN)
	for n, i := range o.Inputs {
		switch i.Context.CodecType {
		case avcodec.AVMEDIA_TYPE_AUDIO, avcodec.AVMEDIA_TYPE_VIDEO:
			if codecType == avcodec.AVMEDIA_TYPE_UNKNOWN {
				codecType = i.Context.CodecType
			} else if codecType != i.Context.CodecType {
				err = fmt.Errorf("astilibav: codec type %d of input %s is different from chosen codec type %d", i.Context.CodecType, n, codecType)
				return
			}
		default:
			err = fmt.Errorf("astilibav: codec type %v is not handled by filterer", i.Context.CodecType)
			return
		}
	}

	// Create buffer func and buffer sink
	var bufferFunc func() *avfilter.Filter
	var bufferSink *avfilter.Filter
	switch codecType {
	case avcodec.AVMEDIA_TYPE_AUDIO:
		bufferFunc = func() *avfilter.Filter { return avfilter.AvfilterGetByName("abuffer") }
		bufferSink = avfilter.AvfilterGetByName("abuffersink")
	case avcodec.AVMEDIA_TYPE_VIDEO:
		bufferFunc = func() *avfilter.Filter { return avfilter.AvfilterGetByName("buffer") }
		bufferSink = avfilter.AvfilterGetByName("buffersink")
	}

	// Create buffer sink ctx
	var bufferSinkCtx *avfilter.Context
	if ret := avfilter.AvfilterGraphCreateFilter(&bufferSinkCtx, bufferSink, "out", "", nil, g); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: avfilter.AvfilterGraphCreateFilter on empty args failed")
		return
	}

	// Create inputs
	inputs := avfilter.AvfilterInoutAlloc()
	inputs.SetName("out")
	inputs.SetFilterCtx(bufferSinkCtx)
	inputs.SetPadIdx(0)
	inputs.SetNext(nil)

	// Loop through options inputs
	var previousOutput *avfilter.Input
	bufferSrcCtxs := make(map[astiencoder.Node]*avfilter.Context)
	for n, i := range o.Inputs {
		// Create buffer
		bufferSrc := bufferFunc()

		// Create filter in args
		var args string
		switch i.Context.CodecType {
		case avcodec.AVMEDIA_TYPE_AUDIO:
			args = fmt.Sprintf("channel_layout=%s:sample_fmt=%s:time_base=%d/%d:sample_rate=%d", avutil.AvGetChannelLayoutString(i.Context.ChannelLayout), avutil.AvGetSampleFmtName(int(i.Context.SampleFmt)), i.Context.TimeBase.Num(), i.Context.TimeBase.Den(), i.Context.SampleRate)
		case avcodec.AVMEDIA_TYPE_VIDEO:
			args = fmt.Sprintf("video_size=%dx%d:pix_fmt=%d:time_base=%d/%d:pixel_aspect=%d/%d", i.Context.Width, i.Context.Height, i.Context.PixelFormat, i.Context.TimeBase.Num(), i.Context.TimeBase.Den(), i.Context.SampleAspectRatio.Num(), i.Context.SampleAspectRatio.Den())
		default:
			err = fmt.Errorf("astilibav: codec type %v is not handled by filterer", i.Context.CodecType)
			return
		}

		// Create ctx
		var bufferSrcCtx *avfilter.Context
		if ret := avfilter.AvfilterGraphCreateFilter(&bufferSrcCtx, bufferSrc, "in", args, nil, g); ret < 0 {
			err = errors.Wrapf(NewAvError(ret), "astilibav: avfilter.AvfilterGraphCreateFilter on args %s failed", args)
			return
		}

		// Create outputs
		outputs := avfilter.AvfilterInoutAlloc()
		outputs.SetName(n)
		outputs.SetFilterCtx(bufferSrcCtx)
		outputs.SetPadIdx(0)
		outputs.SetNext(previousOutput)

		// Store ctx
		bufferSrcCtxs[i.Node] = bufferSrcCtx

		// Set previous output
		previousOutput = outputs
	}

	// Parse content
	if ret := g.AvfilterGraphParsePtr(o.Content, &inputs, &previousOutput, nil); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: g.AvfilterGraphParsePtr on content %s failed", o.Content)
		return
	}

	// Configure
	if ret := g.AvfilterGraphConfig(nil); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: g.AvfilterGraphConfig failed")
		return
	}

	// Create filterer
	return NewFilterer(bufferSrcCtxs, bufferSinkCtx, g, e, c), nil
}

func (f *Filterer) addStats() {
	// Add incoming rate
	f.Stater().AddStat(astistat.StatMetadata{
		Description: "Number of frames coming in per second",
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
	// Add handler
	f.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(f, h)
}

// Disconnect implements the FrameHandlerConnector interface
func (f *Filterer) Disconnect(h FrameHandler) {
	// Delete handler
	f.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(f, h)
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

			// Retrieve buffer ctx
			bufferSrcCtx, ok := f.bufferSrcCtxs[p.Node]
			if !ok {
				return
			}

			// Push frame in graph
			f.statWorkRatio.Add(true)
			if ret := f.g.AvBuffersrcAddFrameFlags(bufferSrcCtx, p.Frame, avfilter.AV_BUFFERSRC_FLAG_KEEP_REF); ret < 0 {
				f.statWorkRatio.Done(true)
				emitAvError(f.e, ret, "f.g.AvBuffersrcAddFrameFlags failed")
				return
			}
			f.statWorkRatio.Done(true)

			// Loop
			for {
				// Pull filtered frame
				if stop := f.pullFilteredFrame(p.Descriptor); stop {
					return
				}
			}
		})
	})
}

func (f *Filterer) pullFilteredFrame(descriptor Descriptor) (stop bool) {
	// Get frame
	fm := f.d.p.get()
	defer f.d.p.put(fm)

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
	f.d.dispatch(fm, newFiltererDescriptor(f.bufferSinkCtx, descriptor))
	return
}

// HandleFrame implements the FrameHandler interface
func (f *Filterer) HandleFrame(p *FrameHandlerPayload) {
	f.q.Send(p)
}

// SendCommand sends a command to the filterer
func (f *Filterer) SendCommand(target, cmd, arg string, flags int) (err error) {
	var res string
	if ret := f.g.AvfilterGraphSendCommand(target, cmd, arg, res, 255, flags); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: f.g.AvfilterGraphSendCommand for target %s, cmd %s, arg %s and flag %d failed with res %s", target, cmd, arg, flags, res)
		return
	}
	return
}

type filtererDescriptor struct {
	timeBase avutil.Rational
}

func newFiltererDescriptor(bufferSinkCtx *avfilter.Context, prev Descriptor) (d *filtererDescriptor) {
	d = &filtererDescriptor{}
	if is := bufferSinkCtx.Inputs(); is != nil && len(is) > 0 {
		d.timeBase = is[0].TimeBase()
	} else {
		d.timeBase = prev.TimeBase()
	}
	return
}

// TimeBase implements the Descriptor interface
func (d *filtererDescriptor) TimeBase() avutil.Rational {
	return d.timeBase
}
