package astilibav

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countFilterer uint64

// Filterer represents an object capable of applying a filter to frames
type Filterer struct {
	*astiencoder.BaseNode
	buffersinkContext *astiav.FilterContext
	buffersrcContexts map[astiencoder.Node][]*astiav.FilterContext
	c                 *astikit.Chan
	d                 *frameDispatcher
	eh                *astiencoder.EventHandler
	emulatePeriod     time.Duration
	g                 *astiav.FilterGraph
	outputCtx         Context
	p                 *framePool
	restamper         FrameRestamper
	statIncomingRate  *astikit.CounterRateStat
	statProcessedRate *astikit.CounterRateStat
}

// FiltererOptions represents filterer options
type FiltererOptions struct {
	Content     string
	EmulateRate astiav.Rational
	Inputs      map[string]astiencoder.Node
	Node        astiencoder.NodeOptions
	OutputCtx   Context
	Restamper   FrameRestamper
}

// NewFilterer creates a new filterer
func NewFilterer(o FiltererOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (f *Filterer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countFilterer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("filterer_%d", count), fmt.Sprintf("Filterer #%d", count), "Filters", "filterer")

	// Create filterer
	f = &Filterer{
		buffersrcContexts: make(map[astiencoder.Node][]*astiav.FilterContext),
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:                eh,
		outputCtx:         o.OutputCtx,
		restamper:         o.Restamper,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	f.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, f, astiencoder.EventTypeToNodeEventName)

	// Create frame pool
	f.p = newFramePool(f)

	// Create frame dispatcher
	f.d = newFrameDispatcher(f, eh, f.p)

	// Add stats
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
	f.g = astiav.AllocFilterGraph()
	f.AddClose(f.g.Free)

	// Create buffersrc func and buffersink
	var buffersrcFunc func() *astiav.Filter
	var buffersink *astiav.Filter
	switch o.OutputCtx.MediaType {
	case astiav.MediaTypeAudio:
		buffersrcFunc = func() *astiav.Filter { return astiav.FindFilterByName("abuffer") }
		buffersink = astiav.FindFilterByName("abuffersink")
	case astiav.MediaTypeVideo:
		buffersrcFunc = func() *astiav.Filter { return astiav.FindFilterByName("buffer") }
		buffersink = astiav.FindFilterByName("buffersink")
	default:
		err = fmt.Errorf("astilibav: media type %s is not handled by filterer", o.OutputCtx.MediaType)
		return
	}

	// No buffersink
	if buffersink == nil {
		err = errors.New("astilibav: buffersink is nil")
		return
	}

	// Create buffersink context
	if f.buffersinkContext, err = f.g.NewFilterContext(buffersink, "out", nil); err != nil {
		err = fmt.Errorf("astilibav: creating buffersink context failed: %w", err)
		return
	}

	// Make sure buffersink context is freed
	f.AddClose(f.buffersinkContext.Free)

	// Create inputs
	inputs := astiav.AllocFilterInOut()
	f.AddClose(inputs.Free)
	inputs.SetName("out")
	inputs.SetFilterContext(f.buffersinkContext)
	inputs.SetPadIdx(0)
	inputs.SetNext(nil)

	// Loop through options inputs
	var outputs *astiav.FilterInOut
	defer func() {
		if outputs != nil {
			outputs.Free()
		}
	}()
	for n, i := range o.Inputs {
		// Get context
		v, ok := i.(OutputContexter)
		if !ok {
			err = fmt.Errorf("astilibav: input %s is not an OutputContexter", n)
			return
		}
		ctx := v.OutputCtx()

		// Create buffersrc
		buffersrc := buffersrcFunc()

		// No buffersrc
		if buffersrc == nil {
			err = errors.New("astilibav: buffersrc is nil")
			return
		}

		// Create args
		var args astiav.FilterArgs
		switch ctx.MediaType {
		case astiav.MediaTypeAudio:
			args = astiav.FilterArgs{
				"sample_fmt":  ctx.SampleFormat.String(),
				"sample_rate": strconv.Itoa(ctx.SampleRate),
				"time_base":   ctx.TimeBase.String(),
			}
			if ctx.Channels > 0 {
				args["channel_layout"] = ctx.ChannelLayout.StringWithNbChannels(ctx.Channels)
			} else {
				args["channel_layout"] = ctx.ChannelLayout.String()
			}
		case astiav.MediaTypeVideo:
			args = astiav.FilterArgs{
				"pix_fmt":      strconv.Itoa(int(ctx.PixelFormat)),
				"pixel_aspect": ctx.SampleAspectRatio.String(),
				"time_base":    ctx.TimeBase.String(),
				"video_size":   strconv.Itoa(ctx.Width) + "x" + strconv.Itoa(ctx.Height),
			}
			if ctx.FrameRate.Num() > 0 {
				args["frame_rate"] = ctx.FrameRate.String()
			}
		default:
			err = fmt.Errorf("astilibav: media type %s is not handled by filterer", ctx.MediaType)
			return
		}

		// Create buffersrc ctx
		var buffersrcCtx *astiav.FilterContext
		if buffersrcCtx, err = f.g.NewFilterContext(buffersrc, "in", args); err != nil {
			err = fmt.Errorf("astilibav: creating buffersrc context failed: %w", err)
			return
		}

		// Make sure buffersrc context is freed
		f.AddClose(buffersrcCtx.Free)

		// Create outputs
		o := astiav.AllocFilterInOut()
		o.SetName(n)
		o.SetFilterContext(buffersrcCtx)
		o.SetPadIdx(0)
		o.SetNext(outputs)

		// Store ctx
		f.buffersrcContexts[i] = append(f.buffersrcContexts[i], buffersrcCtx)

		// Set outputs
		outputs = o
	}

	// Parse filter
	if err = f.g.Parse(o.Content, inputs, outputs); err != nil {
		err = fmt.Errorf("astilibav: parsing filter failed: %w", err)
		return
	}

	// Configure filter
	if err = f.g.Configure(); err != nil {
		err = fmt.Errorf("astilibav: configuring filter failed: %w", err)
		return
	}
	return
}

func (f *Filterer) addStats() {
	// Get stats
	ss := f.c.Stats()
	ss = append(ss, f.d.stats()...)
	ss = append(ss,
		astikit.StatOptions{
			Handler: f.statIncomingRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
		},
		astikit.StatOptions{
			Handler: f.statProcessedRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
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
		if len(f.buffersrcContexts) == 0 {
			nextAt := time.Now()
			desc := newFiltererDescriptor(f.buffersinkContext, nil)
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
		astikit.Sleep(f.Context(), delta) //nolint:errcheck
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
func (f *Filterer) HandleFrame(p FrameHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	f.DoWhenUnclosed(func() {
		// Increment incoming rate
		f.statIncomingRate.Add(1)

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

				// Make sure to close frame
				defer f.p.put(fm)

				// Increment processed rate
				f.statProcessedRate.Add(1)

				// Retrieve buffer ctxs
				buffersrcContexts, ok := f.buffersrcContexts[p.Node]
				if !ok {
					return
				}

				// Loop through buffer ctxs
				for _, buffersrcContext := range buffersrcContexts {
					// Add frame
					if err := buffersrcContext.BuffersrcAddFrame(fm, astiav.NewBuffersrcFlags(astiav.BuffersrcFlagKeepRef)); err != nil {
						emitError(f, f.eh, err, "adding frame to buffersrc")
						return
					}
				}

				// Loop
				for {
					// Pull filtered frame
					if stop := f.pullFilteredFrame(p.Descriptor); stop {
						return
					}
				}
			})
		})
	})
}

func (f *Filterer) pullFilteredFrame(descriptor Descriptor) (stop bool) {
	// Get frame
	fm := f.p.get()
	defer f.p.put(fm)

	// Pull filtered frame from graph
	if err := f.buffersinkContext.BuffersinkGetFrame(fm, astiav.NewBuffersinkFlags()); err != nil {
		if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
			emitError(f, f.eh, err, "getting frame from buffersink")
		}
		stop = true
		return
	}

	// Restamp
	if f.restamper != nil {
		f.restamper.Restamp(fm)
	}

	// Dispatch frame
	f.d.dispatch(fm, newFiltererDescriptor(f.buffersinkContext, descriptor))
	return
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

func newFiltererDescriptor(buffersinkContext *astiav.FilterContext, prev Descriptor) (d *filtererDescriptor) {
	d = &filtererDescriptor{}
	if is := buffersinkContext.Inputs(); len(is) > 0 {
		d.timeBase = is[0].TimeBase()
	} else {
		d.timeBase = prev.TimeBase()
	}
	return
}

// TimeBase implements the Descriptor interface
func (d *filtererDescriptor) TimeBase() astiav.Rational {
	return d.timeBase
}
