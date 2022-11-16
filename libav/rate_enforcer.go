package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countRateEnforcer uint64

// RateEnforcer represents an object capable of enforcing rate based on PTS
type RateEnforcer struct {
	*astiencoder.BaseNode
	c                   *astikit.Chan
	currentNode         astiencoder.Node
	d                   *frameDispatcher
	delay               time.Duration
	descriptor          Descriptor
	desiredNode         astiencoder.Node
	eh                  *astiencoder.EventHandler
	f                   RateEnforcerFiller
	frames              map[astiencoder.Node][]*astiav.Frame
	m                   *sync.Mutex
	outputCtx           Context
	p                   *framePool
	period              time.Duration
	ptsReferences       map[astiencoder.Node]*rateEnforcerPTSReference
	restamper           FrameRestamper
	statFramesDelay     *astikit.AtomicDuration
	statFramesFilled    uint64
	statFramesProcessed uint64
	statFramesReceived  uint64
}

type rateEnforcerPTSReference struct {
	pts      int64
	t        time.Time
	timeBase astiav.Rational
}

func newRateEnforcerPTSReference(pts int64, t time.Time, timeBase astiav.Rational) *rateEnforcerPTSReference {
	return &rateEnforcerPTSReference{
		pts:      pts,
		t:        t,
		timeBase: timeBase,
	}
}

func (r *rateEnforcerPTSReference) timeFromPTS(pts int64) time.Time {
	return r.t.Add(time.Duration(astiav.RescaleQ(pts-r.pts, r.timeBase, nanosecondRational)))
}

func (r *rateEnforcerPTSReference) ptsFromTime(t time.Time) int64 {
	return astiav.RescaleQ(int64(t.Sub(r.t)), nanosecondRational, r.timeBase) + r.pts
}

// RateEnforcerOptions represents rate enforcer options
type RateEnforcerOptions struct {
	Delay  time.Duration
	Filler RateEnforcerFiller
	Node   astiencoder.NodeOptions
	// Both FrameRate and TimeBase are mandatory
	OutputCtx Context
	Restamper FrameRestamper
}

// NewRateEnforcer creates a new rate enforcer
func NewRateEnforcer(o RateEnforcerOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (r *RateEnforcer) {
	// Extend node metadata
	count := atomic.AddUint64(&countRateEnforcer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("rate_enforcer_%d", count), fmt.Sprintf("Rate Enforcer #%d", count), "Enforces rate", "rate enforcer")

	// Create rate enforcer
	r = &RateEnforcer{
		c:               astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		delay:           o.Delay,
		descriptor:      o.OutputCtx.Descriptor(),
		frames:          make(map[astiencoder.Node][]*astiav.Frame),
		eh:              eh,
		f:               o.Filler,
		m:               &sync.Mutex{},
		outputCtx:       o.OutputCtx,
		period:          time.Duration(float64(1e9) / o.OutputCtx.FrameRate.ToDouble()),
		ptsReferences:   map[astiencoder.Node]*rateEnforcerPTSReference{},
		restamper:       o.Restamper,
		statFramesDelay: astikit.NewAtomicDuration(0),
	}

	// Create base node
	r.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, r, astiencoder.EventTypeToNodeEventName)

	// Create frame pool
	r.p = newFramePool(r)

	// Create frame dispatcher
	r.d = newFrameDispatcher(r, eh)

	// Create filler
	if r.f == nil {
		r.f = newPreviousRateEnforcerFiller(r, r.eh, r.p)
	}

	// Add stat options
	r.addStatOptions()
	return
}

type RateEnforcerStats struct {
	FramesAllocated uint64
	FramesDelay     time.Duration
	FramesDispached uint64
	FramesFilled    uint64
	FramesProcessed uint64
	FramesReceived  uint64
	WorkDuration    time.Duration
}

func (r *RateEnforcer) Stats() RateEnforcerStats {
	return RateEnforcerStats{
		FramesAllocated: r.p.stats().framesAllocated,
		FramesDelay:     r.statFramesDelay.Duration(),
		FramesDispached: r.d.stats().framesDispatched,
		FramesFilled:    atomic.LoadUint64(&r.statFramesFilled),
		FramesProcessed: atomic.LoadUint64(&r.statFramesProcessed),
		FramesReceived:  atomic.LoadUint64(&r.statFramesReceived),
		WorkDuration:    r.c.Stats().WorkDuration,
	}
}

func (r *RateEnforcer) addStatOptions() {
	// Get stats
	ss := r.c.StatOptions()
	ss = append(ss, r.d.statOptions()...)
	ss = append(ss, r.p.statOptions()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&r.statFramesReceived),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&r.statFramesProcessed),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Average delay of frames coming in",
				Label:       "Average delay",
				Name:        StatNameAverageDelay,
				Unit:        "ns",
			},
			Valuer: astikit.NewAtomicDurationAvgStat(r.statFramesDelay, &r.statFramesProcessed),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames filled per second",
				Label:       "Filled rate",
				Name:        StatNameFilledRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&r.statFramesFilled),
		},
	)

	// Add stats
	r.BaseNode.AddStats(ss...)
}

// OutputCtx returns the output ctx
func (r *RateEnforcer) OutputCtx() Context {
	return r.outputCtx
}

// Switch switches the source
func (r *RateEnforcer) Switch(n astiencoder.Node) {
	r.m.Lock()
	defer r.m.Unlock()
	r.desiredNode = n
}

// Connect implements the FrameHandlerConnector interface
func (r *RateEnforcer) Connect(h FrameHandler) {
	// Add handler
	r.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(r, h)
}

// Disconnect implements the FrameHandlerConnector interface
func (r *RateEnforcer) Disconnect(h FrameHandler) {
	// Delete handler
	r.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(r, h)
}

// Start starts the rate enforcer
func (r *RateEnforcer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	r.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to stop the chan properly
		defer r.c.Stop()

		// Start tick in a goroutine
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			// Make sure to update the waiting group
			defer wg.Done()

			// Start tick
			r.startTick(r.Context())
		}()

		// Start chan
		r.c.Start(r.Context())

		// Wait for start tick to be really over
		wg.Wait()
	})
}

// HandleFrame implements the FrameHandler interface
func (r *RateEnforcer) HandleFrame(p FrameHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	r.DoWhenUnclosed(func() {
		// Increment received frames
		atomic.AddUint64(&r.statFramesReceived, 1)

		// Invalid pts
		if p.Frame.Pts() == astiav.NoPtsValue {
			return
		}

		// Copy frame
		f := r.p.get()
		if err := f.Ref(p.Frame); err != nil {
			emitError(r, r.eh, err, "refing frame")
			return
		}

		// Get time
		t := time.Now().Add(r.delay)

		// Restamp
		f.SetPts(astiav.RescaleQ(f.Pts(), p.Descriptor.TimeBase(), r.outputCtx.TimeBase))

		// Add to chan
		r.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			r.DoWhenUnclosed(func() {
				// Handle pause
				defer r.HandlePause()

				// Increment processed frames
				atomic.AddUint64(&r.statFramesProcessed, 1)

				// Lock
				r.m.Lock()
				defer r.m.Unlock()

				// Insert frame
				var inserted bool
				for idx, v := range r.frames[p.Node] {
					if f.Pts() > v.Pts() {
						continue
					}
					if f.Pts() == v.Pts() {
						r.p.put(f)
						return
					} else {
						r.frames[p.Node] = append(r.frames[p.Node][:idx], append([]*astiav.Frame{f}, r.frames[p.Node][idx:]...)...)
					}
					inserted = true
					break
				}

				// Frame was not inserted, we need to append it
				if !inserted {
					r.frames[p.Node] = append(r.frames[p.Node], f)
				}

				// Update pts reference
				ptsReference, ok := r.ptsReferences[p.Node]
				if !ok || ptsReference == nil || ptsReference.timeFromPTS(f.Pts()).After(t) {
					r.ptsReferences[p.Node] = newRateEnforcerPTSReference(f.Pts(), t, r.outputCtx.TimeBase)
					ptsReference = r.ptsReferences[p.Node]
				}

				// Increment frames delay
				if r.currentNode == p.Node {
					r.statFramesDelay.Add(t.Sub(ptsReference.timeFromPTS(f.Pts())))
				}
			})
		})
	})
}

func (r *RateEnforcer) startTick(ctx context.Context) {
	nextAt := time.Now()
	for {
		if stop := r.tickFunc(ctx, &nextAt); stop {
			return
		}
	}
}

func (r *RateEnforcer) tickFunc(ctx context.Context, nextAt *time.Time) (stop bool) {
	// Compute next at
	*nextAt = nextAt.Add(r.period)

	// Sleep until next at
	if delta := time.Until(*nextAt); delta > 0 {
		astikit.Sleep(ctx, delta) //nolint:errcheck
	}

	// Check context
	if ctx.Err() != nil {
		return true
	}

	// Lock
	r.m.Lock()
	defer r.m.Unlock()

	// Get frame
	f, n, filled := r.frame(*nextAt)

	// Process frame
	if f != nil {
		// Restamp frame
		if r.restamper != nil {
			r.restamper.Restamp(f)
		}

		// Dispatch frame
		r.d.dispatch(f, r.descriptor)

		// Frame is coming from an actual node
		if n != nil {
			// New node has been dispatched
			if r.currentNode != n {
				// Emit event
				r.eh.Emit(astiencoder.Event{
					Name:    EventNameRateEnforcerSwitchedOut,
					Payload: n,
					Target:  r,
				})

				// Update current node
				r.currentNode = n
			}
		}
	}

	// Frame has been filled
	if filled {
		atomic.AddUint64(&r.statFramesFilled, 1)
	} else {
		r.p.put(f)
	}
	return
}

func (r *RateEnforcer) frame(from time.Time) (f *astiav.Frame, n astiencoder.Node, filled bool) {
	// Get to
	to := from.Add(r.period)

	// If desired node is different from the current node, we check it first
	if r.desiredNode != nil && r.desiredNode != r.currentNode {
		if f = r.frameForNode(r.desiredNode, from, to); f != nil {
			n = r.desiredNode
		}
	}

	// No frame, we need to check the current node if any
	if f == nil && r.currentNode != nil {
		if f = r.frameForNode(r.currentNode, from, to); f != nil {
			n = r.currentNode
		}
	}

	// Cleanup
	r.cleanup(to)

	// Fill
	if f == nil {
		f, n = r.f.Fill()
		filled = true
	} else {
		r.f.NoFill(f, n)
	}
	return
}

func (r *RateEnforcer) frameForNode(n astiencoder.Node, from, to time.Time) (f *astiav.Frame) {
	// Get pts reference
	ptsReference, ok := r.ptsReferences[n]
	if !ok {
		return
	}

	// Get pts boundaries
	ptsMin := ptsReference.ptsFromTime(from)
	ptsMax := ptsReference.ptsFromTime(to)

	// Loop through frames
	for idx := range r.frames[n] {
		if r.frames[n][idx].Pts() >= ptsMin && r.frames[n][idx].Pts() < ptsMax {
			f = r.frames[n][idx]
			r.frames[n] = append(r.frames[n][:idx], r.frames[n][idx+1:]...)
			break
		}
	}
	return
}

func (r *RateEnforcer) cleanup(to time.Time) {
	// Loop through nodes
	for n := range r.frames {
		// Get pts reference
		ptsReference, ok := r.ptsReferences[n]
		if !ok {
			continue
		}

		// Get max pts
		ptsMax := ptsReference.ptsFromTime(to)

		// Loop through frames
		for idx := 0; idx < len(r.frames[n]); idx++ {
			// PTS is too old
			if r.frames[n][idx].Pts() < ptsMax {
				r.p.put(r.frames[n][idx])
				r.frames[n] = append(r.frames[n][:idx], r.frames[n][idx+1:]...)
				idx--
			}
		}
	}
}

type RateEnforcerFiller interface {
	Fill() (*astiav.Frame, astiencoder.Node)
	NoFill(*astiav.Frame, astiencoder.Node)
}

type previousRateEnforcerFiller struct {
	eh     *astiencoder.EventHandler
	f      *astiav.Frame
	n      astiencoder.Node
	p      *framePool
	target interface{}
}

func newPreviousRateEnforcerFiller(target interface{}, eh *astiencoder.EventHandler, p *framePool) *previousRateEnforcerFiller {
	return &previousRateEnforcerFiller{
		eh:     eh,
		p:      p,
		target: target,
	}
}

func (f *previousRateEnforcerFiller) Fill() (*astiav.Frame, astiencoder.Node) {
	return f.f, f.n
}

func (f *previousRateEnforcerFiller) NoFill(fm *astiav.Frame, n astiencoder.Node) {
	// Store
	f.n = n

	// Create frame
	if f.f == nil {
		f.f = f.p.get()
	} else {
		f.f.Unref()
	}

	// Copy frame
	if err := f.f.Ref(fm); err != nil {
		emitError(f.target, f.eh, err, "refing frame")
		f.p.put(f.f)
		f.f = nil
	}
}

type frameRateEnforcerFiller struct {
	f *astiav.Frame
}

func NewFrameRateEnforcerFiller(fn func(fm *astiav.Frame) error, c *astikit.Closer) (f *frameRateEnforcerFiller, err error) {
	// Alloc frame
	fm := astiav.AllocFrame()

	// Make sure to free frame
	defer func(err *error) {
		if *err != nil {
			fm.Free()
		} else {
			c.Add(fm.Free)
		}
	}(&err)

	// Adapt frame
	if fn != nil {
		if err = fn(fm); err != nil {
			err = fmt.Errorf("astilibav: adapting frame failed: %w", err)
			return
		}
	}

	// Create filler
	f = &frameRateEnforcerFiller{
		f: fm,
	}
	return
}

func (f *frameRateEnforcerFiller) Fill() (*astiav.Frame, astiencoder.Node) {
	return f.f, nil
}

func (f *frameRateEnforcerFiller) NoFill(fm *astiav.Frame, n astiencoder.Node) {}
