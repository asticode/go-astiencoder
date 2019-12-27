package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
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
	c                *astikit.Chan
	cl               *astikit.Closer
	ccl              *astikit.Closer // Child closer used to close only things related to the filterer
	d                *frameDispatcher
	eh               *astiencoder.EventHandler
	g                *avfilter.Graph
	restamper        FrameRestamper
	s                FiltererSwitcher
	statIncomingRate *astikit.CounterAvgStat
	statWorkRatio    *astikit.DurationPercentageStat
}

// FiltererOptions represents filterer options
type FiltererOptions struct {
	Content   string
	Inputs    map[string]FiltererInput
	Node      astiencoder.NodeOptions
	Restamper FrameRestamper
	Switcher  FiltererSwitcher
}

// FiltererInput represents a filterer input
type FiltererInput struct {
	Context Context
	Node    astiencoder.Node
}

// NewFilterer creates a new filterer
func NewFilterer(o FiltererOptions, eh *astiencoder.EventHandler, c *astikit.Closer) (f *Filterer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countFilterer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("filterer_%d", count), fmt.Sprintf("Filterer #%d", count), "Filters")

	// Create filterer
	f = &Filterer{
		bufferSrcCtxs: make(map[astiencoder.Node]*avfilter.Context),
		c: astikit.NewChan(astikit.ChanOptions{
			AddStrategy: astikit.ChanAddStrategyBlockWhenStarted,
			ProcessAll:  true,
		}),
		cl:               c,
		ccl:              c.NewChild(),
		eh:               eh,
		g:                avfilter.AvfilterGraphAlloc(),
		restamper:        o.Restamper,
		s:                o.Switcher,
		statIncomingRate: astikit.NewCounterAvgStat(),
		statWorkRatio:    astikit.NewDurationPercentageStat(),
	}
	f.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(f), eh)
	f.d = newFrameDispatcher(f, eh, f.ccl)
	f.addStats()

	// We need a filterer switcher
	if f.s == nil {
		f.s = newFiltererSwitcher(f, eh)
	}

	// Create graph
	f.ccl.Add(func() error {
		f.g.AvfilterGraphFree()
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
	// We need to create an intermediate variable to avoid "cgo argument has Go pointer to Go pointer" errors
	var bufferSinkCtx *avfilter.Context
	if ret := avfilter.AvfilterGraphCreateFilter(&bufferSinkCtx, bufferSink, "out", "", nil, f.g); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: avfilter.AvfilterGraphCreateFilter on empty args failed")
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
		if ret := avfilter.AvfilterGraphCreateFilter(&bufferSrcCtx, bufferSrc, "in", args, nil, f.g); ret < 0 {
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
		f.bufferSrcCtxs[i.Node] = bufferSrcCtx

		// Set previous output
		previousOutput = outputs
	}

	// Parse content
	if ret := f.g.AvfilterGraphParsePtr(o.Content, &inputs, &previousOutput, nil); ret < 0 {
		err = errors.Wrapf(NewAvError(ret), "astilibav: g.AvfilterGraphParsePtr on content %s failed", o.Content)
		return
	}

	// Configure
	if ret := f.g.AvfilterGraphConfig(nil); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: g.AvfilterGraphConfig failed")
		return
	}
	return
}

func (f *Filterer) addStats() {
	// Add incoming rate
	f.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of frames coming in per second",
		Label:       "Incoming rate",
		Unit:        "fps",
	}, f.statIncomingRate)

	// Add work ratio
	f.Stater().AddStat(astikit.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, f.statWorkRatio)

	// Add dispatcher stats
	f.d.addStats(f.Stater())

	// Add queue stats
	f.c.AddStats(f.Stater())
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

		// Make sure to stop the queue properly
		defer f.c.Stop()

		// Reset switcher
		if f.s != nil {
			f.s.Reset()
		}

		// Start queue
		f.c.Start(f.Context())
	})
}

// HandleFrame implements the FrameHandler interface
func (f *Filterer) HandleFrame(p *FrameHandlerPayload) {
	f.c.Add(func() {
		// Handle pause
		defer f.HandlePause()

		// Increment incoming rate
		f.statIncomingRate.Add(1)

		// Retrieve buffer ctx
		bufferSrcCtx, ok := f.bufferSrcCtxs[p.Node]
		if !ok {
			return
		}

		// Check switcher
		if f.s != nil {
			if ko := f.s.ShouldIn(p.Node); ko {
				return
			}
		}

		// Push frame in graph
		f.statWorkRatio.Begin()
		if ret := f.g.AvBuffersrcAddFrameFlags(bufferSrcCtx, p.Frame, avfilter.AV_BUFFERSRC_FLAG_KEEP_REF); ret < 0 {
			f.statWorkRatio.End()
			emitAvError(f, f.eh, ret, "f.g.AvBuffersrcAddFrameFlags failed")
			return
		}
		f.statWorkRatio.End()

		// Increment switcher
		if f.s != nil {
			f.s.IncIn(p.Node)
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

	// Check switcher
	if f.s != nil {
		if stop = f.s.ShouldOut(); stop {
			return
		}
	}

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

	// Increment switcher
	if f.s != nil {
		f.s.IncOut()
	}

	// Dispatch frame
	f.d.dispatch(fm, newFiltererDescriptor(f.bufferSinkCtx, descriptor))
	return
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

// FiltererSwitchOptions represents filterer switch options
type FiltererSwitchOptions struct {
	Filter   FiltererOptions
	Workflow *astiencoder.Workflow
}

// Switch disconnects/stops/closes the current filterer and starts/connects the new filterer properly
func (f *Filterer) Switch(opt FiltererSwitchOptions) (nf *Filterer, err error) {
	// Create next filterer
	if nf, err = NewFilterer(opt.Filter, f.eh, f.cl); err != nil {
		err = errors.Wrap(err, "astilibav: creating new filterer failed")
		return
	}

	// Connect next filterer to previous filterer's children
	for _, c := range f.Children() {
		nf.Connect(c.(FrameHandler))
	}

	// Start next filterer
	opt.Workflow.StartNodes(nf)

	// Index previous filterer nodes
	pns := make(map[astiencoder.Node]bool)
	for _, n := range f.Parents() {
		pns[n] = true
	}

	// Index next filterer nodes
	nns := make(map[astiencoder.Node]bool)
	for _, i := range opt.Filter.Inputs {
		nns[i.Node] = true
	}

	// Handle out
	f.eh.Add(f, EventNameFiltererSwitchOutDone, func(e astiencoder.Event) bool {
		// Disconnect previous filterer's children
		for _, c := range f.Children() {
			f.Disconnect(c.(FrameHandler))
		}

		// Make sure to close the previous filter once stopped
		f.eh.Add(f, astiencoder.EventNameNodeStopped, func(e astiencoder.Event) bool {
			if err := f.ccl.Close(); err != nil {
				f.eh.Emit(astiencoder.EventError(f, errors.Wrap(err, "astilibav: closing filterer failed")))
			}
			return true
		})

		// Stop previous filterer
		f.Stop()
		return true
	})

	// Handle in
	o := &sync.Once{}
	f.eh.Add(f, EventNameFiltererSwitchInDone, func(e astiencoder.Event) bool {
		// Assert node
		n := e.Payload.(astiencoder.Node)
		c := e.Payload.(FrameHandlerConnector)

		// Disconnect node
		c.Disconnect(f)

		// Connect node if part of next filterer's inputs
		if _, ok := nns[n]; ok {
			c.Connect(nf)
		}

		// Connect other nodes only once
		o.Do(func() {
			for n := range nns {
				if _, ok := pns[n]; !ok {
					n.(FrameHandlerConnector).Connect(nf)
				}
			}
		})
		return len(f.Parents()) == 0
	})

	// Switch
	f.s.Switch()
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
