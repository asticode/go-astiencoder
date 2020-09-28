package astilibav

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avfilter"
	"github.com/asticode/goav/avutil"
)

var countFilterer uint64

// Filterer represents an object capable of applying a filter to frames
type Filterer struct {
	*astiencoder.BaseNode
	bufferSinkCtx    *avfilter.Context
	bufferSrcCtxs    map[astiencoder.Node][]*avfilter.Context
	c                *astikit.Chan
	cl               *astikit.Closer
	d                *frameDispatcher
	eh               *astiencoder.EventHandler
	emulatePeriod    time.Duration
	g                *avfilter.Graph
	outputCtx        Context
	restamper        FrameRestamper
	statIncomingRate *astikit.CounterRateStat
	statWorkRatio    *astikit.DurationPercentageStat
}

// FiltererOptions represents filterer options
type FiltererOptions struct {
	Content     string
	EmulateRate avutil.Rational
	Inputs      map[string]astiencoder.Node
	Node        astiencoder.NodeOptions
	OutputCtx   Context
	Restamper   FrameRestamper
}

// NewFilterer creates a new filterer
func NewFilterer(o FiltererOptions, eh *astiencoder.EventHandler, c *astikit.Closer) (f *Filterer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countFilterer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("filterer_%d", count), fmt.Sprintf("Filterer #%d", count), "Filters", "filterer")

	// Create filterer
	f = &Filterer{
		bufferSrcCtxs: make(map[astiencoder.Node][]*avfilter.Context),
		c: astikit.NewChan(astikit.ChanOptions{
			AddStrategy: astikit.ChanAddStrategyBlockWhenStarted,
			ProcessAll:  true,
		}),
		cl:               c.NewChild(),
		eh:               eh,
		g:                avfilter.AvfilterGraphAlloc(),
		outputCtx:        o.OutputCtx,
		restamper:        o.Restamper,
		statIncomingRate: astikit.NewCounterRateStat(),
		statWorkRatio:    astikit.NewDurationPercentageStat(),
	}
	f.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(f), eh)
	f.d = newFrameDispatcher(f, eh, f.cl)
	f.addStats()

	// No inputs
	if len(o.Inputs) == 0 {
		// No emulate rate
		if o.EmulateRate.Num() <= 0 || o.EmulateRate.Den() <= 0 {
			err = errors.New("astilibav: no inputs but no emulate rate either")
			return
		}

		// Get emulate period
		f.emulatePeriod = time.Duration(o.EmulateRate.Den() * 1e9 / o.EmulateRate.Num())
	}

	// Create graph
	f.cl.Add(func() error {
		f.g.AvfilterGraphFree()
		return nil
	})

	// Create buffer func and buffer sink
	var bufferFunc func() *avfilter.Filter
	var bufferSink *avfilter.Filter
	switch o.OutputCtx.CodecType {
	case avcodec.AVMEDIA_TYPE_AUDIO:
		bufferFunc = func() *avfilter.Filter { return avfilter.AvfilterGetByName("abuffer") }
		bufferSink = avfilter.AvfilterGetByName("abuffersink")
	case avcodec.AVMEDIA_TYPE_VIDEO:
		bufferFunc = func() *avfilter.Filter { return avfilter.AvfilterGetByName("buffer") }
		bufferSink = avfilter.AvfilterGetByName("buffersink")
	default:
		err = fmt.Errorf("astilibav: codec type %v is not handled by filterer", o.OutputCtx.CodecType)
		return
	}

	// Create buffer sink ctx
	// We need to create an intermediate variable to avoid "cgo argument has Go pointer to Go pointer" errors
	var bufferSinkCtx *avfilter.Context
	if ret := avfilter.AvfilterGraphCreateFilter(&bufferSinkCtx, bufferSink, "out", "", nil, f.g); ret < 0 {
		err = fmt.Errorf("astilibav: avfilter.AvfilterGraphCreateFilter on empty args failed: %w", NewAvError(ret))
		return
	}
	f.bufferSinkCtx = bufferSinkCtx

	// Create inputs
	inputs := avfilter.AvfilterInoutAlloc()
	inputs.SetName("out")
	inputs.SetFilterCtx(f.bufferSinkCtx)
	inputs.SetPadIdx(0)
	inputs.SetNext(nil)

	// Loop through options inputs
	var previousOutput *avfilter.Input
	for n, i := range o.Inputs {
		// Get context
		v, ok := i.(OutputContexter)
		if !ok {
			err = fmt.Errorf("astilibav: input %s is not an OutputContexter", n)
			return
		}
		ctx := v.OutputCtx()

		// Create buffer
		bufferSrc := bufferFunc()

		// Create filter in args
		var args string
		switch ctx.CodecType {
		case avcodec.AVMEDIA_TYPE_AUDIO:
			args = fmt.Sprintf("channel_layout=%s:sample_fmt=%s:time_base=%d/%d:sample_rate=%d", avutil.AvGetChannelLayoutString(ctx.ChannelLayout), avutil.AvGetSampleFmtName(int(ctx.SampleFmt)), ctx.TimeBase.Num(), ctx.TimeBase.Den(), ctx.SampleRate)
		case avcodec.AVMEDIA_TYPE_VIDEO:
			args = fmt.Sprintf("video_size=%dx%d:pix_fmt=%d:time_base=%d/%d:pixel_aspect=%d/%d", ctx.Width, ctx.Height, ctx.PixelFormat, ctx.TimeBase.Num(), ctx.TimeBase.Den(), ctx.SampleAspectRatio.Num(), ctx.SampleAspectRatio.Den())
			if ctx.FrameRate.Num() > 0 {
				args += fmt.Sprintf(":frame_rate=%d/%d", ctx.FrameRate.Num(), ctx.FrameRate.Den())
			}
		default:
			err = fmt.Errorf("astilibav: codec type %v is not handled by filterer", ctx.CodecType)
			return
		}

		// Create ctx
		var bufferSrcCtx *avfilter.Context
		if ret := avfilter.AvfilterGraphCreateFilter(&bufferSrcCtx, bufferSrc, "in", args, nil, f.g); ret < 0 {
			err = fmt.Errorf("astilibav: avfilter.AvfilterGraphCreateFilter on args %s failed: %w", args, NewAvError(ret))
			return
		}

		// Create outputs
		outputs := avfilter.AvfilterInoutAlloc()
		outputs.SetName(n)
		outputs.SetFilterCtx(bufferSrcCtx)
		outputs.SetPadIdx(0)
		outputs.SetNext(previousOutput)

		// Store ctx
		f.bufferSrcCtxs[i] = append(f.bufferSrcCtxs[i], bufferSrcCtx)

		// Set previous output
		previousOutput = outputs
	}

	// Parse content
	if ret := f.g.AvfilterGraphParsePtr(o.Content, &inputs, &previousOutput, nil); ret < 0 {
		err = fmt.Errorf("astilibav: g.AvfilterGraphParsePtr on content %s failed: %w", o.Content, NewAvError(ret))
		return
	}

	// Configure
	if ret := f.g.AvfilterGraphConfig(nil); ret < 0 {
		err = fmt.Errorf("astilibav: g.AvfilterGraphConfig failed: %w", NewAvError(ret))
		return
	}
	return
}

func (f *Filterer) Close() error {
	return f.cl.Close()
}

func (f *Filterer) addStats() {
	// Add incoming rate
	f.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of frames coming in per second",
		Label:       "Incoming rate",
		Name:        StatNameIncomingRate,
		Unit:        "fps",
	}, f.statIncomingRate)

	// Add work ratio
	f.Stater().AddStat(astikit.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Name:        StatNameWorkRatio,
		Unit:        "%",
	}, f.statWorkRatio)

	// Add dispatcher stats
	f.d.addStats(f.Stater())

	// Add queue stats
	f.c.AddStats(f.Stater())
}

// OutputCtx returns the output ctx
func (f *Filterer) OutputCtx() Context {
	return f.outputCtx
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
	f.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer f.d.wait()

		// In case there are no inputs, we emulate frames coming in
		if len(f.bufferSrcCtxs) == 0 {
			nextAt := time.Now()
			desc := newFiltererDescriptor(f.bufferSinkCtx, nil)
			for {
				if stop := f.tickFunc(&nextAt, desc); stop {
					break
				}
			}
			return
		}

		// Make sure to stop the queue properly
		defer f.c.Stop()

		// Start queue
		f.c.Start(f.Context())
	})
}

func (f *Filterer) tickFunc(nextAt *time.Time, desc Descriptor) (stop bool) {
	// Compute next at
	*nextAt = nextAt.Add(f.emulatePeriod)

	// Sleep until next at
	if delta := time.Until(*nextAt); delta > 0 {
		astikit.Sleep(f.Context(), delta)
	}

	// Check context
	if f.Context().Err() != nil {
		stop = true
		return
	}

	// Pull filtered frame
	f.pullFilteredFrame(desc)
	return
}

// HandleFrame implements the FrameHandler interface
func (f *Filterer) HandleFrame(p *FrameHandlerPayload) {
	f.c.Add(func() {
		// Handle pause
		defer f.HandlePause()

		// Increment incoming rate
		f.statIncomingRate.Add(1)

		// Retrieve buffer ctxs
		bufferSrcCtxs, ok := f.bufferSrcCtxs[p.Node]
		if !ok {
			return
		}

		// Loop through buffer ctxs
		for _, bufferSrcCtx := range bufferSrcCtxs {
			// Push frame in graph
			f.statWorkRatio.Begin()
			if ret := f.g.AvBuffersrcAddFrameFlags(bufferSrcCtx, p.Frame, avfilter.AV_BUFFERSRC_FLAG_KEEP_REF); ret < 0 {
				f.statWorkRatio.End()
				emitAvError(f, f.eh, ret, "f.g.AvBuffersrcAddFrameFlags failed")
				return
			}
			f.statWorkRatio.End()
		}

		// Loop
		for {
			// Pull filtered frame
			if stop := f.pullFilteredFrame(p.Descriptor); stop {
				return
			}
		}
	})
}

func (f *Filterer) pullFilteredFrame(descriptor Descriptor) (stop bool) {
	// Get frame
	fm := f.d.p.get()
	defer f.d.p.put(fm)

	// Pull filtered frame from graph
	f.statWorkRatio.Begin()
	if ret := f.g.AvBuffersinkGetFrame(f.bufferSinkCtx, fm); ret < 0 {
		f.statWorkRatio.End()
		if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
			emitAvError(f, f.eh, ret, "f.g.AvBuffersinkGetFrame failed")
		}
		stop = true
		return
	}
	f.statWorkRatio.End()

	// Restamp
	if f.restamper != nil {
		f.statWorkRatio.Begin()
		f.restamper.Restamp(fm)
		f.statWorkRatio.End()
	}

	// Dispatch frame
	f.d.dispatch(fm, newFiltererDescriptor(f.bufferSinkCtx, descriptor))
	return
}

// SendCommand sends a command to the filterer
func (f *Filterer) SendCommand(target, cmd, arg string, flags int) (err error) {
	var res string
	if ret := f.g.AvfilterGraphSendCommand(target, cmd, arg, res, 255, flags); ret < 0 {
		err = fmt.Errorf("astilibav: f.g.AvfilterGraphSendCommand for target %s, cmd %s, arg %s and flag %d failed with res %s: %w", target, cmd, arg, flags, res, NewAvError(ret))
		return
	}
	return
}

type filtererDescriptor struct {
	timeBase avutil.Rational
}

func newFiltererDescriptor(bufferSinkCtx *avfilter.Context, prev Descriptor) (d *filtererDescriptor) {
	d = &filtererDescriptor{}
	if is := bufferSinkCtx.Inputs(); len(is) > 0 {
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
