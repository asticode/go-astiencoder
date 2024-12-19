package astilibav

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

type FiltererFrameHandlerStrategy string

const (
	FiltererFrameHandlerStrategyDefault FiltererFrameHandlerStrategy = "default"
	FiltererFrameHandlerStrategyPTS     FiltererFrameHandlerStrategy = "pts"
)

var countFilterer uint64

// Filterer represents an object capable of applying a filter to frames
type Filterer struct {
	*astiencoder.BaseNode
	buffersinkContext     *astiav.BuffersinkFilterContext
	c                     *astikit.Chan
	content               string
	d                     *frameDispatcher
	eh                    *astiencoder.EventHandler
	emulatePeriod         time.Duration
	g                     *astiav.FilterGraph
	gc                    *astikit.Closer
	h                     filtererFrameHandler
	inputs                map[astiencoder.Node]*filtererInput
	items                 []*filtererItem
	onInputContextChanges FiltererOnInputContextChangesFunc
	outputCtx             Context
	p                     *framePool
	restamper             FrameRestamper
	statFramesProcessed   uint64
	statFramesReceived    uint64
	threadCount           int
	threadType            astiav.ThreadType
}

type filtererFrame struct {
	f *astiav.Frame
	i *filtererInput
	n astiencoder.Node
}

type filtererFrameHandler interface {
	add(f *filtererFrame, src []*filtererItem) (dst []*filtererItem)
	onPulled(f *astiav.Frame) (dispatch bool)
	onReady(i *filtererItem)
}

type filtererInput struct {
	ctx           Context
	buffersrcCtxs []*astiav.BuffersrcFilterContext
	name          string
}

type filtererItem struct {
	fs     map[astiencoder.Node]*filtererFrame
	opaque interface{}
}

type FiltererOnInputContextChangesFunc func(cs FiltererInputContextChanges, f *Filterer) (ignore bool)

// FiltererOptions represents filterer options
type FiltererOptions struct {
	Content               string
	EmulateRate           astiav.Rational
	FrameHandlerStrategy  FiltererFrameHandlerStrategy
	Inputs                map[string]astiencoder.Node
	Node                  astiencoder.NodeOptions
	OnInputContextChanges FiltererOnInputContextChangesFunc
	OutputCtx             Context
	Restamper             FrameRestamper
	ThreadCount           int
	ThreadType            astiav.ThreadType
}

// NewFilterer creates a new filterer
func NewFilterer(o FiltererOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (f *Filterer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countFilterer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("filterer_%d", count), fmt.Sprintf("Filterer #%d", count), "Filters", "filterer")

	// Create filterer
	f = &Filterer{
		c:                     astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		content:               o.Content,
		eh:                    eh,
		inputs:                make(map[astiencoder.Node]*filtererInput),
		onInputContextChanges: o.OnInputContextChanges,
		outputCtx:             o.OutputCtx,
		restamper:             o.Restamper,
		threadCount:           o.ThreadCount,
		threadType:            o.ThreadType,
	}

	// Create base node
	f.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, f, astiencoder.EventTypeToNodeEventName)

	// Create frame pool
	f.p = newFramePool(f.NewChildCloser())

	// Create frame dispatcher
	f.d = newFrameDispatcher(f, eh)

	// Create inputs
	for name, n := range o.Inputs {
		f.inputs[n] = &filtererInput{name: name}
	}

	// Switch on frame handler strategy
	switch {
	case o.FrameHandlerStrategy == FiltererFrameHandlerStrategyPTS && len(f.inputs) > 0:
		// Create frame handler
		f.h = newPTSFiltererFrameHandler()
	default:
		// Create frame handler
		f.h = newDefaultFiltererFrameHandler()
	}

	// Make sure to close items
	f.AddClose(func() {
		for _, i := range f.items {
			for _, v := range i.fs {
				f.p.put(v.f)
			}
		}
	})

	// Add stat options
	f.addStatOptions()

	// No inputs
	if len(f.inputs) == 0 {
		// No emulate rate
		if o.EmulateRate.Num() <= 0 || o.EmulateRate.Den() <= 0 {
			err = errors.New("astilibav: no inputs but no emulate rate either")
			return
		}

		// Get emulate period
		f.emulatePeriod = time.Duration(o.EmulateRate.Den() * 1e9 / o.EmulateRate.Num())
	}

	// Get graph contexts
	var ctxs map[astiencoder.Node]Context
	if ctxs, err = f.defaultGraphContexts(); err != nil {
		err = fmt.Errorf("astilibav: getting graph contexts failed: %w", err)
		return
	}

	// Create graph
	if err = f.createGraph(ctxs); err != nil {
		err = fmt.Errorf("astilibav: creating graph failed: %w", err)
		return
	}

	// Make sure to close graph
	f.AddClose(func() {
		if f.gc != nil {
			f.gc.Close()
			f.gc = nil
		}
	})
	return
}

func (f *Filterer) defaultGraphContexts() (ctxs map[astiencoder.Node]Context, err error) {
	// Create contexts
	ctxs = make(map[astiencoder.Node]Context)

	// Loop through inputs
	for n, i := range f.inputs {
		// Get context
		v, ok := n.(OutputContexter)
		if !ok {
			err = fmt.Errorf("astilibav: input %s is not an OutputContexter", i.name)
			return
		}

		// Store context
		ctxs[n] = v.OutputCtx()
	}
	return
}

func (f *Filterer) graphContextsFromItem(i *filtererItem) (ctxs map[astiencoder.Node]Context) {
	// Create contexts
	ctxs = make(map[astiencoder.Node]Context)

	// Loop through frames
	for n, fm := range i.fs {
		// Create context
		ctx := fm.i.ctx
		switch ctx.MediaType {
		case astiav.MediaTypeAudio:
			ctx.ChannelLayout = fm.f.ChannelLayout()
			ctx.SampleFormat = fm.f.SampleFormat()
			ctx.SampleRate = fm.f.SampleRate()
		case astiav.MediaTypeVideo:
			ctx.Height = fm.f.Height()
			ctx.PixelFormat = fm.f.PixelFormat()
			ctx.SampleAspectRatio = fm.f.SampleAspectRatio()
			ctx.Width = fm.f.Width()
		}

		// Store context
		ctxs[n] = ctx
	}
	return
}

func (f *Filterer) createGraph(ctxs map[astiencoder.Node]Context) (err error) {
	// Make sure to close previous graph
	if f.gc != nil {
		f.gc.Close()
		f.gc = nil
	}

	// Create closer
	c := astikit.NewCloser()

	// Create graph
	g := astiav.AllocFilterGraph()
	c.Add(g.Free)

	// Set thread parameters
	if f.threadCount > 0 {
		g.SetThreadCount(f.threadCount)
	}
	if f.threadType != astiav.ThreadTypeUndefined {
		g.SetThreadType(f.threadType)
	}

	// Create buffersrc func and buffersink
	var buffersrcFunc func() *astiav.Filter
	var buffersink *astiav.Filter
	switch f.outputCtx.MediaType {
	case astiav.MediaTypeAudio:
		buffersrcFunc = func() *astiav.Filter { return astiav.FindFilterByName("abuffer") }
		buffersink = astiav.FindFilterByName("abuffersink")
	case astiav.MediaTypeVideo:
		buffersrcFunc = func() *astiav.Filter { return astiav.FindFilterByName("buffer") }
		buffersink = astiav.FindFilterByName("buffersink")
	default:
		err = fmt.Errorf("astilibav: media type %s is not handled by filterer", f.outputCtx.MediaType)
		return
	}

	// No buffersink
	if buffersink == nil {
		err = errors.New("astilibav: buffersink is nil")
		return
	}

	// Create buffersink context
	var buffersinkContext *astiav.BuffersinkFilterContext
	if buffersinkContext, err = g.NewBuffersinkFilterContext(buffersink, "out"); err != nil {
		err = fmt.Errorf("astilibav: creating buffersink context failed: %w", err)
		return
	}

	// Make sure buffersink context is freed
	c.Add(buffersinkContext.FilterContext().Free)

	// Create inputs
	inputs := astiav.AllocFilterInOut()
	c.Add(inputs.Free)
	inputs.SetName("out")
	inputs.SetFilterContext(buffersinkContext.FilterContext())
	inputs.SetPadIdx(0)
	inputs.SetNext(nil)

	// Loop through provided inputs
	type inputUpdate struct {
		ctx  Context
		ctxs []*astiav.BuffersrcFilterContext
		i    *filtererInput
	}
	inputUpdates := make(map[astiencoder.Node]*inputUpdate)
	var outputs *astiav.FilterInOut
	defer func() {
		if outputs != nil {
			outputs.Free()
		}
	}()
	for n, i := range f.inputs {
		// Create buffersrc
		buffersrc := buffersrcFunc()

		// No buffersrc
		if buffersrc == nil {
			err = errors.New("astilibav: buffersrc is nil")
			return
		}

		// Get context
		ctx, ok := ctxs[n]
		if !ok {
			err = fmt.Errorf("astilibav: no context for %s", i.name)
			return
		}

		// Create buffersrc context parameters
		buffersrcCtxParameters := astiav.AllocBuffersrcFilterContextParameters()
		defer buffersrcCtxParameters.Free()
		switch ctx.MediaType {
		case astiav.MediaTypeAudio:
			buffersrcCtxParameters.SetChannelLayout(ctx.ChannelLayout)
			buffersrcCtxParameters.SetSampleFormat(ctx.SampleFormat)
			buffersrcCtxParameters.SetSampleRate(ctx.SampleRate)
			buffersrcCtxParameters.SetTimeBase(ctx.TimeBase)
		case astiav.MediaTypeVideo:
			buffersrcCtxParameters.SetHeight(ctx.Height)
			buffersrcCtxParameters.SetPixelFormat(ctx.PixelFormat)
			buffersrcCtxParameters.SetSampleAspectRatio(ctx.SampleAspectRatio)
			buffersrcCtxParameters.SetTimeBase(ctx.TimeBase)
			buffersrcCtxParameters.SetWidth(ctx.Width)
			if ctx.FrameRate.Num() > 0 {
				buffersrcCtxParameters.SetFramerate(ctx.FrameRate)
			}
		default:
			err = fmt.Errorf("astilibav: media type %s is not handled by filterer", ctx.MediaType)
			return
		}

		// Create buffersrc ctx
		var buffersrcCtx *astiav.BuffersrcFilterContext
		if buffersrcCtx, err = g.NewBuffersrcFilterContext(buffersrc, "in"); err != nil {
			err = fmt.Errorf("astilibav: creating buffersrc context failed: %w", err)
			return
		}

		// Make sure buffersrc context is freed
		c.Add(buffersrcCtx.FilterContext().Free)

		// Set buffersrc context parameters
		if err = buffersrcCtx.SetParameters(buffersrcCtxParameters); err != nil {
			err = fmt.Errorf("main: setting buffersrc context parameters failed: %w", err)
			return
		}

		// Initialize buffersrc context
		if err = buffersrcCtx.Initialize(); err != nil {
			err = fmt.Errorf("main: initializing buffersrc context failed: %w", err)
			return
		}

		// Create outputs
		o := astiav.AllocFilterInOut()
		o.SetName(i.name)
		o.SetFilterContext(buffersrcCtx.FilterContext())
		o.SetPadIdx(0)
		o.SetNext(outputs)

		// Make sure input update exists
		if _, ok := inputUpdates[n]; !ok {
			inputUpdates[n] = &inputUpdate{
				ctx: ctx,
				i:   i,
			}
		}

		// Store ctx
		inputUpdates[n].ctxs = append(inputUpdates[n].ctxs, buffersrcCtx)

		// Set outputs
		outputs = o
	}

	// Parse filter
	if err = g.Parse(f.content, inputs, outputs); err != nil {
		err = fmt.Errorf("astilibav: parsing filter failed: %w", err)
		return
	}

	// Configure filter
	if err = g.Configure(); err != nil {
		err = fmt.Errorf("astilibav: configuring filter failed: %w", err)
		return
	}

	// Update filterer
	f.buffersinkContext = buffersinkContext
	for _, u := range inputUpdates {
		u.i.ctx = u.ctx
		u.i.buffersrcCtxs = u.ctxs
	}
	f.g = g
	f.gc = c
	return
}

type FiltererStats struct {
	FramesAllocated  uint64
	FramesDispatched uint64
	FramesProcessed  uint64
	FramesReceived   uint64
	WorkDuration     time.Duration
}

func (f *Filterer) Stats() FiltererStats {
	return FiltererStats{
		FramesAllocated:  f.p.stats().framesAllocated,
		FramesDispatched: f.d.stats().framesDispatched,
		FramesProcessed:  atomic.LoadUint64(&f.statFramesProcessed),
		FramesReceived:   atomic.LoadUint64(&f.statFramesReceived),
		WorkDuration:     f.c.Stats().WorkDuration,
	}
}

func (f *Filterer) addStatOptions() {
	// Get stats
	ss := f.c.StatOptions()
	ss = append(ss, f.d.statOptions()...)
	ss = append(ss, f.p.statOptions()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&f.statFramesReceived),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&f.statFramesProcessed),
		},
	)

	// Add stats
	f.BaseNode.AddStats(ss...)
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
		// In case there are no inputs, we emulate frames coming in
		if len(f.inputs) == 0 {
			nextAt := time.Now()
			for {
				if stop := f.tickFunc(&nextAt); stop {
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

func (f *Filterer) tickFunc(nextAt *time.Time) (stop bool) {
	// Compute next at
	*nextAt = nextAt.Add(f.emulatePeriod)

	// Sleep until next at
	if delta := time.Until(*nextAt); delta > 0 {
		astikit.Sleep(f.Context(), delta) //nolint:errcheck
	}

	// Check context
	if f.Context().Err() != nil {
		stop = true
		return
	}

	// Pull filtered frame
	f.pullFilteredFrame()
	return
}

// HandleFrame implements the FrameHandler interface
func (f *Filterer) HandleFrame(p FrameHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	f.DoWhenUnclosed(func() {
		// Increment received frames
		atomic.AddUint64(&f.statFramesReceived, 1)

		// Copy frame
		fm := f.p.get()
		if err := fm.Ref(p.Frame); err != nil {
			emitError(f, f.eh, err, "refing frame")
			return
		}

		// Add to chan
		f.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			f.DoWhenUnclosed(func() {
				// Handle pause
				defer f.HandlePause()

				// Increment processed frames
				atomic.AddUint64(&f.statFramesProcessed, 1)

				// Get input
				input, ok := f.inputs[p.Node]
				if !ok {
					return
				}

				// Add frame
				f.items = f.h.add(&filtererFrame{
					f: fm,
					i: input,
					n: p.Node,
				}, f.items)

				// Get first idx for which all frames are there
				addIdx := -1
				for idx := 0; idx < len(f.items); idx++ {
					if len(f.items[idx].fs) == len(f.inputs) {
						addIdx = idx
						break
					}
				}

				// Not enough frames
				if addIdx == -1 {
					return
				}

				// Get item
				i := f.items[addIdx]

				// Get frames to add
				var framesToAdd []*filtererFrame
				for _, f := range i.fs {
					framesToAdd = append(framesToAdd, f)
				}

				// Get frames to close
				var framesToClose []*filtererFrame
				for idx := 0; idx <= addIdx; idx++ {
					for _, f := range f.items[idx].fs {
						framesToClose = append(framesToClose, f)
					}
				}

				// Make sure to close frames
				defer func() {
					for _, v := range framesToClose {
						f.p.put(v.f)
					}
				}()

				// Item is ready
				f.h.onReady(i)

				// Remove items
				f.items = f.items[addIdx+1:]

				// Process input contexts change
				if err := f.processInputContextsChange(i); err != nil {
					// TODO Fill output frame when frame handler strategy is pts?
					emitError(f, f.eh, err, "processing input contexts change")
					return
				}

				// Loop through frames to add
				for _, v := range framesToAdd {
					// Loop through buffer ctxs
					for _, buffersrcCtx := range v.i.buffersrcCtxs {
						// Add frame
						if err := buffersrcCtx.AddFrame(v.f, astiav.NewBuffersrcFlags(astiav.BuffersrcFlagKeepRef)); err != nil {
							// TODO Fill intput frame when frame handler strategy is pts?
							emitError(f, f.eh, err, "adding frame to buffersrc")
							continue
						}
					}
				}

				// Loop
				for {
					// Pull filtered frame
					if stop := f.pullFilteredFrame(); stop {
						return
					}
				}
			})
		})
	})
}

type FiltererInputContextChanges map[astiencoder.Node]FiltererInputContextChange

func (cs FiltererInputContextChanges) changed() bool {
	for _, c := range cs {
		if c.changed() {
			return true
		}
	}
	return false
}

func (cs FiltererInputContextChanges) String() string {
	var ss []string
	for n, c := range cs {
		if !c.changed() {
			continue
		}
		ss = append(ss, fmt.Sprintf("%s: %s", n.Metadata().Name, c))
	}
	return strings.Join(ss, " | ")
}

type FiltererInputContextChange struct {
	ctx Context
	f   *astiav.Frame
}

func (c FiltererInputContextChange) changed() bool {
	// Switch on media type
	switch c.ctx.MediaType {
	case astiav.MediaTypeAudio:
		if !c.ctx.ChannelLayout.Equal(c.f.ChannelLayout()) ||
			c.ctx.SampleFormat != c.f.SampleFormat() ||
			c.ctx.SampleRate != c.f.SampleRate() {
			return true
		}
	case astiav.MediaTypeVideo:
		if c.ctx.Height != c.f.Height() ||
			c.ctx.PixelFormat != c.f.PixelFormat() ||
			c.ctx.SampleAspectRatio != c.f.SampleAspectRatio() ||
			c.ctx.Width != c.f.Width() {
			return true
		}
	}
	return false
}

func (c FiltererInputContextChange) String() string {
	// Switch on media type
	var ss []string
	switch c.ctx.MediaType {
	case astiav.MediaTypeAudio:
		if !c.ctx.ChannelLayout.Equal(c.f.ChannelLayout()) {
			ss = append(ss, fmt.Sprintf("channel layout changed: %s --> %s", c.ctx.ChannelLayout, c.f.ChannelLayout()))
		}
		if c.ctx.SampleFormat != c.f.SampleFormat() {
			ss = append(ss, fmt.Sprintf("sample format changed: %s --> %s", c.ctx.SampleFormat, c.f.SampleFormat()))
		}
		if c.ctx.SampleRate != c.f.SampleRate() {
			ss = append(ss, fmt.Sprintf("sample rate changed: %d --> %d", c.ctx.SampleRate, c.f.SampleRate()))
		}
	case astiav.MediaTypeVideo:
		if c.ctx.Height != c.f.Height() {
			ss = append(ss, fmt.Sprintf("height changed: %d --> %d", c.ctx.Height, c.f.Height()))
		}
		if c.ctx.PixelFormat != c.f.PixelFormat() {
			ss = append(ss, fmt.Sprintf("pixel format changed: %s --> %s", c.ctx.PixelFormat, c.f.PixelFormat()))
		}
		if c.ctx.SampleAspectRatio != c.f.SampleAspectRatio() {
			ss = append(ss, fmt.Sprintf("sample aspect ratio changed: %s --> %s", c.ctx.SampleAspectRatio, c.f.SampleAspectRatio()))
		}
		if c.ctx.Width != c.f.Width() {
			ss = append(ss, fmt.Sprintf("width changed: %d --> %d", c.ctx.Width, c.f.Width()))
		}
	}
	return strings.Join(ss, " && ")
}

func (f *Filterer) processInputContextsChange(i *filtererItem) (err error) {
	// Create changes
	cs := make(FiltererInputContextChanges)
	for n, f := range i.fs {
		cs[n] = FiltererInputContextChange{
			ctx: f.i.ctx,
			f:   f.f,
		}
	}

	// Nothing changed
	if !cs.changed() {
		return
	}

	// Callback
	if f.onInputContextChanges != nil {
		if ignore := f.onInputContextChanges(cs, f); ignore {
			return
		}
	}

	// Create graph
	if err = f.createGraph(f.graphContextsFromItem(i)); err != nil {
		err = fmt.Errorf("astilibav: creating graph failed: %w", err)
		return
	}
	return
}

func (f *Filterer) pullFilteredFrame() (stop bool) {
	// Get frame
	fm := f.p.get()
	defer f.p.put(fm)

	// Pull filtered frame from graph
	if err := f.buffersinkContext.GetFrame(fm, astiav.NewBuffersinkFlags()); err != nil {
		if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
			// TODO Fill output frame when frame handler strategy is pts?
			emitError(f, f.eh, err, "getting frame from buffersink")
		}
		stop = true
		return
	}

	// On pulled
	f.onPulled(fm)
	return
}

func (f *Filterer) onPulled(fm *astiav.Frame) {
	// On pulled
	if dispatch := f.h.onPulled(fm); !dispatch {
		return
	}

	// Restamp
	if f.restamper != nil {
		f.restamper.Restamp(fm)
	}

	// Dispatch frame
	f.d.dispatch(fm, f.newFiltererDescriptor())
}

// SendCommand sends a command to the filterer
func (f *Filterer) SendCommand(target, cmd, args string, fs astiav.FilterCommandFlags) (resp string, err error) {
	// Everything executed outside the main loop should be protected from the closer
	f.DoWhenUnclosed(func() {
		// Send command
		if resp, err = f.g.SendCommand(target, cmd, args, fs); err != nil {
			err = fmt.Errorf("astilibav: sending command to filter graph failed with response %s: %w", resp, err)
			return
		}
	})
	return
}

type filtererDescriptor struct {
	timeBase astiav.Rational
}

func (f *Filterer) newFiltererDescriptor() (d *filtererDescriptor) {
	return &filtererDescriptor{timeBase: f.buffersinkContext.TimeBase()}
}

// TimeBase implements the Descriptor interface
func (d *filtererDescriptor) TimeBase() astiav.Rational {
	return d.timeBase
}

type defaultFiltererFrameHandler struct{}

func newDefaultFiltererFrameHandler() *defaultFiltererFrameHandler {
	return &defaultFiltererFrameHandler{}
}

func (h *defaultFiltererFrameHandler) add(f *filtererFrame, src []*filtererItem) []*filtererItem {
	// Create new item
	ni := &filtererItem{fs: map[astiencoder.Node]*filtererFrame{f.n: f}}

	// Append
	var inserted bool
	for idx := 0; idx < len(src); idx++ {
		i := src[idx]
		if _, ok := i.fs[f.n]; ok {
			continue
		}
		i.fs[f.n] = f
		inserted = true
		break
	}

	// Frame was not inserted, we need to append it
	if !inserted {
		src = append(src, ni)
	}
	return src
}

func (h *defaultFiltererFrameHandler) onReady(i *filtererItem) {}

func (h *defaultFiltererFrameHandler) onPulled(f *astiav.Frame) (dispatch bool) { return true }

type ptsFiltererFrameHandler struct {
	addedPTS []int64
}

func newPTSFiltererFrameHandler() *ptsFiltererFrameHandler {
	return &ptsFiltererFrameHandler{}
}

func (h *ptsFiltererFrameHandler) add(f *filtererFrame, src []*filtererItem) []*filtererItem {
	// Create new item
	ni := &filtererItem{
		fs:     map[astiencoder.Node]*filtererFrame{f.n: f},
		opaque: f.f.Pts(),
	}

	// Append
	var inserted bool
	for idx := 0; idx < len(src); idx++ {
		i := src[idx]
		pts := i.opaque.(int64)
		if f.f.Pts() > pts {
			continue
		}
		if f.f.Pts() == pts {
			i.fs[f.n] = f
		} else {
			src = append(src[:idx], append([]*filtererItem{ni}, src[idx:]...)...)
		}
		inserted = true
		break
	}

	// Frame was not inserted, we need to append it
	if !inserted {
		src = append(src, ni)
	}
	return src
}

func (h *ptsFiltererFrameHandler) onReady(i *filtererItem) {
	h.addedPTS = append(h.addedPTS, i.opaque.(int64))
}

func (h *ptsFiltererFrameHandler) onPulled(f *astiav.Frame) (dispatch bool) {
	// Get pts
	if len(h.addedPTS) == 0 {
		return
	}

	// Restamp
	f.SetPts(h.addedPTS[0])

	// Remove pts
	h.addedPTS = h.addedPTS[1:]
	return true
}
