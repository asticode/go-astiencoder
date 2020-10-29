package astilibav

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avutil"
)

var countRateEnforcer uint64

// RateEnforcer represents an object capable of enforcing rate based on PTS
type RateEnforcer struct {
	*astiencoder.BaseNode
	buf               []*rateEnforcerItem
	c                 *astikit.Chan
	d                 *frameDispatcher
	eh                *astiencoder.EventHandler
	m                 *sync.Mutex
	n                 astiencoder.Node
	outputCtx         Context
	p                 *framePool
	period            time.Duration
	previousItem      *rateEnforcerItem
	restamper         FrameRestamper
	slotsCount        int
	slots             []*rateEnforcerSlot
	statDelayAvg      *astikit.CounterAvgStat
	statIncomingRate  *astikit.CounterRateStat
	statProcessedRate *astikit.CounterRateStat
	statRepeatedRate  *astikit.CounterRateStat
	timeBase          avutil.Rational
}

type rateEnforcerSlot struct {
	i      *rateEnforcerItem
	n      astiencoder.Node
	ptsMax int64
	ptsMin int64
}

type rateEnforcerItem struct {
	d Descriptor
	f *avutil.Frame
	n astiencoder.Node
}

// RateEnforcerOptions represents rate enforcer options
type RateEnforcerOptions struct {
	// This is expressed in number of frames in the desired FrameRate
	Delay     uint
	FrameRate avutil.Rational
	Node      astiencoder.NodeOptions
	OutputCtx Context
	Restamper FrameRestamper
}

// NewRateEnforcer creates a new rate enforcer
func NewRateEnforcer(o RateEnforcerOptions, eh *astiencoder.EventHandler, c *astikit.Closer) (r *RateEnforcer) {
	// Extend node metadata
	count := atomic.AddUint64(&countRateEnforcer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("rate_enforcer_%d", count), fmt.Sprintf("Rate Enforcer #%d", count), "Enforces rate", "rate enforcer")

	// Create rate enforcer
	r = &RateEnforcer{
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:                eh,
		m:                 &sync.Mutex{},
		outputCtx:         o.OutputCtx,
		p:                 newFramePool(c),
		period:            time.Duration(float64(1e9) / o.FrameRate.ToDouble()),
		restamper:         o.Restamper,
		slots:             []*rateEnforcerSlot{nil},
		slotsCount:        int(math.Max(float64(o.Delay), 1)),
		statDelayAvg:      astikit.NewCounterAvgStat(),
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
		statRepeatedRate:  astikit.NewCounterRateStat(),
		timeBase:          avutil.NewRational(o.FrameRate.Den(), o.FrameRate.Num()),
	}
	r.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(r), eh)
	r.d = newFrameDispatcher(r, eh, r.p)
	r.addStats()
	return
}

func (r *RateEnforcer) addStats() {
	// Add delay avg
	r.Stater().AddStat(astikit.StatMetadata{
		Description: "Average delay of frames coming in",
		Label:       "Average delay",
		Name:        StatNameAverageDelay,
		Unit:        "ms",
	}, r.statDelayAvg)

	// Add incoming rate
	r.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of frames coming in per second",
		Label:       "Incoming rate",
		Name:        StatNameIncomingRate,
		Unit:        "fps",
	}, r.statIncomingRate)

	// Add processed rate
	r.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of frames processed per second",
		Label:       "Processed rate",
		Name:        StatNameProcessedRate,
		Unit:        "fps",
	}, r.statProcessedRate)

	// Add repeated rate
	r.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of frames repeated per second",
		Label:       "Repeated rate",
		Name:        StatNameRepeatedRate,
		Unit:        "fps",
	}, r.statRepeatedRate)

	// Add dispatcher stats
	r.d.addStats(r.Stater())

	// Add chan stats
	r.c.AddStats(r.Stater())
}

// OutputCtx returns the output ctx
func (r *RateEnforcer) OutputCtx() Context {
	return r.outputCtx
}

// Switch switches the source
func (r *RateEnforcer) Switch(n astiencoder.Node) {
	r.m.Lock()
	defer r.m.Unlock()
	r.n = n
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

		// Start tick
		startTickCtx := r.startTick(r.Context())

		// Start chan
		r.c.Start(r.Context())

		// Wait for start tick to be really over since it's not the blocking pattern
		// and is executed in a goroutine
		<-startTickCtx.Done()
	})
}

// HandleFrame implements the FrameHandler interface
func (r *RateEnforcer) HandleFrame(p FrameHandlerPayload) {
	// Increment incoming rate
	r.statIncomingRate.Add(1)

	// Copy frame
	f := r.p.get()
	if ret := avutil.AvFrameRef(f, p.Frame); ret < 0 {
		emitAvError(r, r.eh, ret, "avutil.AvFrameRef failed")
		return
	}

	// Add to chan
	r.c.Add(func() {
		// Handle pause
		defer r.HandlePause()

		// Make sure to close frame
		defer r.p.put(f)

		// Increment processed rate
		r.statProcessedRate.Add(1)

		// Lock
		r.m.Lock()
		defer r.m.Unlock()

		// We update the last slot if:
		//   - there are no slots
		//   - the node of the last slot is different from the desired node AND the payload's node is the desired
		//   node. That way, if the desired node doesn't dispatch frames for some time, we fallback to the previous
		//   node instead of the previous item
		if r.slots[len(r.slots)-1] == nil || (r.n != r.slots[len(r.slots)-1].n && r.n == p.Node) {
			// Create slot
			r.slots[len(r.slots)-1] = r.newRateEnforcerSlot(f, p.Descriptor)

			// Emit event
			r.eh.Emit(astiencoder.Event{
				Name:    EventNameRateEnforcerSwitchedIn,
				Payload: p.Node,
				Target:  r,
			})
		}

		// Create item
		i := r.newRateEnforcerItem(p.Descriptor, p.Node)

		// Copy frame
		i.f = r.p.get()
		if ret := avutil.AvFrameRef(i.f, f); ret < 0 {
			emitAvError(r, r.eh, ret, "avutil.AvFrameRef failed")
			return
		}

		// Append item
		r.buf = append(r.buf, i)

		// Get newest slot for this node
		var s *rateEnforcerSlot
		for _, rs := range r.slots {
			if rs != nil && rs.n == i.n && (s == nil || s.ptsMax < rs.ptsMax) {
				s = rs
			}
		}

		// Process delay stat
		if s != nil {
			r.statDelayAvg.Add(float64(time.Duration(avutil.AvRescaleQ(s.ptsMax-i.f.Pts(), i.d.TimeBase(), nanosecondRational)).Milliseconds()))
		}
	})
}

func (r *RateEnforcer) newRateEnforcerSlot(f *avutil.Frame, d Descriptor) *rateEnforcerSlot {
	return &rateEnforcerSlot{
		n:      r.n,
		ptsMax: f.Pts() + int64(r.timeBase.ToDouble()/d.TimeBase().ToDouble()),
		ptsMin: f.Pts(),
	}
}

func (r *RateEnforcer) newRateEnforcerItem(d Descriptor, n astiencoder.Node) *rateEnforcerItem {
	return &rateEnforcerItem{
		d: d,
		n: n,
	}
}

func (r *RateEnforcer) startTick(parentCtx context.Context) (ctx context.Context) {
	// Create independant context that only captures when the following goroutine ends
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())

	// Execute the rest in a go routine
	go func() {
		// Make sure to cancel local context
		defer cancel()

		// Loop
		nextAt := time.Now()
		var previousNode astiencoder.Node
		for {
			if stop := r.tickFunc(parentCtx, &nextAt, &previousNode); stop {
				return
			}
		}
	}()
	return
}

func (r *RateEnforcer) tickFunc(ctx context.Context, nextAt *time.Time, previousNode *astiencoder.Node) (stop bool) {
	// Compute next at
	*nextAt = nextAt.Add(r.period)

	// Sleep until next at
	if delta := time.Until(*nextAt); delta > 0 {
		astikit.Sleep(ctx, delta)
	}

	// Check context
	if ctx.Err() != nil {
		return true
	}

	// Lock
	r.m.Lock()
	defer r.m.Unlock()

	// Make sure to remove first slot AFTER adding next slot, so that when there's only
	// one slot, we still can get the .next() slot
	removeFirstSlot := true
	defer func(b *bool) {
		if *b {
			r.slots = r.slots[1:]
		}
	}(&removeFirstSlot)

	// Make sure to add next slot
	defer func() {
		var s *rateEnforcerSlot
		if ps := r.slots[len(r.slots)-1]; ps != nil {
			s = ps.next()
		}
		r.slots = append(r.slots, s)
	}()

	// Not enough slots
	if len(r.slots) < r.slotsCount {
		removeFirstSlot = false
		return
	}

	// Distribute
	r.distribute()

	// Dispatch
	i, previous := r.current()
	if i != nil {
		// Restamp frame
		if r.restamper != nil {
			r.restamper.Restamp(i.f)
		}

		// Dispatch frame
		r.d.dispatch(i.f, i.d)

		// New node has been dispatched
		if *previousNode != i.n {
			// Emit event
			r.eh.Emit(astiencoder.Event{
				Name:    EventNameRateEnforcerSwitchedOut,
				Payload: i.n,
				Target:  r,
			})

			// Update previous node
			*previousNode = i.n
		}
	}

	// Frame has been repeated
	if previous {
		r.statRepeatedRate.Add(1)
	} else {
		r.p.put(i.f)
	}
	return
}

func (s *rateEnforcerSlot) next() *rateEnforcerSlot {
	return &rateEnforcerSlot{
		n:      s.n,
		ptsMin: s.ptsMax,
		ptsMax: s.ptsMax - s.ptsMin + s.ptsMax,
	}
}

func (r *RateEnforcer) distribute() {
	// Get useful nodes
	ns := make(map[astiencoder.Node]bool)
	for _, s := range r.slots {
		if s != nil && s.n != nil {
			ns[s.n] = true
		}
	}

	// Loop through slots
	for _, s := range r.slots {
		// Slot is empty or already has an item
		if s == nil || s.i != nil {
			continue
		}

		// Loop through buffer
		for idx := 0; idx < len(r.buf); idx++ {
			// Not the same node
			if r.buf[idx].n != s.n {
				// Node is useless
				if _, ok := ns[r.buf[idx].n]; !ok {
					r.p.put(r.buf[idx].f)
					r.buf = append(r.buf[:idx], r.buf[idx+1:]...)
					idx--
				}
				continue
			}

			// Add to slot or remove if pts is older
			if s.ptsMin <= r.buf[idx].f.Pts() && s.ptsMax > r.buf[idx].f.Pts() {
				if s.i == nil {
					s.i = r.buf[idx]
				} else {
					r.p.put(r.buf[idx].f)
				}
				r.buf = append(r.buf[:idx], r.buf[idx+1:]...)
				idx--
				continue
			} else if s.ptsMin > r.buf[idx].f.Pts() {
				r.p.put(r.buf[idx].f)
				r.buf = append(r.buf[:idx], r.buf[idx+1:]...)
				idx--
				continue
			}
		}
	}
}

func (r *RateEnforcer) debug() (o string) {
	o = "\nSlots:\n"
	for _, s := range r.slots {
		if s != nil {
			o += fmt.Sprintf("min: %d - max: %d - full: %v\n", s.ptsMin, s.ptsMax, s.i != nil)
		}
	}
	o += "\nBuffer:\n"
	for idx, i := range r.buf {
		if i != nil && i.f != nil {
			o += fmt.Sprintf("%d: %d\n", idx, i.f.Pts())
		}
	}
	return
}

func (r *RateEnforcer) current() (i *rateEnforcerItem, previous bool) {
	if r.slots[0] != nil && r.slots[0].i != nil {
		// Get item
		i = r.slots[0].i

		// Create previous item
		if r.previousItem == nil {
			// Create item
			r.previousItem = &rateEnforcerItem{
				d: i.d,
				f: r.p.get(),
				n: i.n,
			}
		} else {
			avutil.AvFrameUnref(r.previousItem.f)
		}

		// Copy frame
		if ret := avutil.AvFrameRef(r.previousItem.f, i.f); ret < 0 {
			emitAvError(r, r.eh, ret, "avutil.AvFrameRef failed")
			r.p.put(r.previousItem.f)
			r.previousItem = nil
		}
	} else {
		i = r.previousItem
		previous = true
	}
	return
}
