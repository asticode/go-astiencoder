package astilibav

import (
	"sync"
	"time"

	"github.com/asticode/go-astiencoder"
)

type Delayer interface {
	Apply(t time.Time) time.Time
	Delay() time.Duration
	HandleFrame(delay time.Duration, n astiencoder.Node)
}

type AdaptiveDelayerOptions struct {
	Handler    AdaptiveDelayerHandlerOptions
	Minimum    time.Duration
	Step       time.Duration
	StepsCount uint
}

type AdaptiveDelayerHandlerOptions struct {
	Average  *AdaptiveDelayerAverageHandlerOptions
	Lossless *AdaptiveDelayerLosslessHandlerOptions
}

type AdaptiveDelayer struct {
	d          time.Duration
	h          adaptiveDelayerHandler
	m          *sync.Mutex
	minimum    time.Duration
	step       time.Duration
	stepsCount int
}

type adaptiveDelayerHandler interface {
	handleFrame(delay time.Duration, n astiencoder.Node)
	updateDelay()
}

func NewAdaptiveDelayer(o AdaptiveDelayerOptions) *AdaptiveDelayer {
	// Create delayer
	d := &AdaptiveDelayer{
		d:          o.Minimum,
		m:          &sync.Mutex{},
		minimum:    o.Minimum,
		step:       o.Step,
		stepsCount: int(o.StepsCount),
	}
	if d.stepsCount < 1 {
		d.stepsCount = 1
	}

	// Create handler
	switch {
	case o.Handler.Average != nil:
		d.h = newAdaptiveDelayerAverageHandler(d, *o.Handler.Average)
	case o.Handler.Lossless != nil:
		d.h = newAdaptiveDelayerLosslessHandler(d, *o.Handler.Lossless)
	}
	return d
}

func (d *AdaptiveDelayer) Apply(t time.Time) time.Time {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Add delay
	return t.Add(-d.delayUnsafe())
}

func (d *AdaptiveDelayer) Delay() time.Duration {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Get delay
	return d.delayUnsafe()
}

func (d *AdaptiveDelayer) delayUnsafe() time.Duration {
	// Make sure to update delay first
	d.updateDelayUnsafe()

	// Get delay
	return d.d
}

func (d *AdaptiveDelayer) updateDelayUnsafe() {
	if d.h != nil {
		d.h.updateDelay()
	}
}

func (d *AdaptiveDelayer) HandleFrame(delay time.Duration, n astiencoder.Node) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Make sure to update delay first
	d.updateDelayUnsafe()

	// Handle frame
	if d.h != nil {
		d.h.handleFrame(delay, n)
	}
}

func (d *AdaptiveDelayer) SetMinimum(min time.Duration) {
	d.minimum = min
}

var _ adaptiveDelayerHandler = (*adaptiveDelayerAverageHandler)(nil)

type adaptiveDelayerAverageHandler struct {
	b              *adaptiveDelayerAverageHandlerBuffer
	d              *AdaptiveDelayer
	lookBehind     time.Duration
	marginDecrease time.Duration
	marginIncrease time.Duration
}

type adaptiveDelayerAverageHandlerBuffer struct {
	delays  map[astiencoder.Node][]time.Duration
	firstAt time.Time
}

func newAdaptiveDelayerAverageHandlerBuffer(n time.Time) *adaptiveDelayerAverageHandlerBuffer {
	return &adaptiveDelayerAverageHandlerBuffer{
		delays:  make(map[astiencoder.Node][]time.Duration),
		firstAt: n,
	}
}

type AdaptiveDelayerAverageHandlerOptions struct {
	LookBehind     time.Duration
	MarginDecrease time.Duration
	MarginIncrease time.Duration
}

func newAdaptiveDelayerAverageHandler(d *AdaptiveDelayer, o AdaptiveDelayerAverageHandlerOptions) *adaptiveDelayerAverageHandler {
	return &adaptiveDelayerAverageHandler{
		d:              d,
		lookBehind:     o.LookBehind,
		marginDecrease: o.MarginDecrease,
		marginIncrease: o.MarginIncrease,
	}
}

func (h *adaptiveDelayerAverageHandler) handleFrame(delay time.Duration, n astiencoder.Node) {
	// Make sure to create buffer
	if h.b == nil {
		h.b = newAdaptiveDelayerAverageHandlerBuffer(now())
	}

	// Add delay
	h.b.delays[n] = append(h.b.delays[n], delay)
}

func (h *adaptiveDelayerAverageHandler) updateDelay() {
	// Nothing to do
	if h.b == nil || time.Since(h.b.firstAt) < h.lookBehind {
		return
	}

	// Get max average delay
	var maxAverageDelay *time.Duration
	for _, delays := range h.b.delays {
		// No delays
		if len(delays) <= 0 {
			continue
		}

		// Get average
		var sum time.Duration
		for _, v := range delays {
			sum += v
		}
		avg := sum / time.Duration(len(delays))

		// Update max average delay
		if maxAverageDelay == nil || *maxAverageDelay < avg {
			maxAverageDelay = &avg
		}
	}

	// Process max average delay
	if maxAverageDelay != nil {
		// Loop through steps
		for idx := 0; idx < h.d.stepsCount; idx++ {
			// Get step delay
			stepDelay := time.Duration(idx)*h.d.step + h.d.minimum

			// Same as current delay
			if h.d.d == stepDelay {
				continue
			}

			// Get boundaries
			var min, max time.Duration
			if stepDelay < h.d.d {
				min = stepDelay - h.d.step - h.marginDecrease
				max = stepDelay - h.marginDecrease
			} else {
				min = stepDelay - h.d.step - h.marginIncrease
				max = stepDelay - h.marginIncrease
			}

			// Update delay
			if (idx == 0 || *maxAverageDelay >= min) && (idx == h.d.stepsCount-1 || *maxAverageDelay <= max) {
				h.d.d = stepDelay
				break
			}
		}
	}

	// Reset buffer
	h.b = nil
}

var _ adaptiveDelayerHandler = (*adaptiveDelayerLosslessHandler)(nil)

type adaptiveDelayerLosslessHandler struct {
	b               map[astiencoder.Node][]adaptiveDelayerLosslessHandlerBufferItem
	d               *AdaptiveDelayer
	disableDecrease bool
	lookBehind      time.Duration
}

type adaptiveDelayerLosslessHandlerBufferItem struct {
	at    time.Time
	delay time.Duration
}

type AdaptiveDelayerLosslessHandlerOptions struct {
	DisableDecrease bool
	LookBehind      time.Duration
}

func newAdaptiveDelayerLosslessHandler(d *AdaptiveDelayer, o AdaptiveDelayerLosslessHandlerOptions) *adaptiveDelayerLosslessHandler {
	h := &adaptiveDelayerLosslessHandler{
		b:               make(map[astiencoder.Node][]adaptiveDelayerLosslessHandlerBufferItem),
		d:               d,
		disableDecrease: o.DisableDecrease,
		lookBehind:      o.LookBehind,
	}
	if h.lookBehind <= 0 {
		h.lookBehind = time.Second
	}
	return h
}

func (h *adaptiveDelayerLosslessHandler) handleFrame(delay time.Duration, n astiencoder.Node) {
	// Add delay
	h.b[n] = append(h.b[n], adaptiveDelayerLosslessHandlerBufferItem{
		at:    now(),
		delay: delay,
	})
}

func (h *adaptiveDelayerLosslessHandler) updateDelay() {
	// Loop through nodes
	var maxDelay time.Duration
	for n := range h.b {
		// Loop through items
		for idx := 0; idx < len(h.b[n]); idx++ {
			// Get item
			i := h.b[n][idx]

			// Item is too old
			if now().Sub(i.at) >= h.lookBehind {
				h.b[n] = append(h.b[n][:idx], h.b[n][idx+1:]...)
				idx--
				continue
			}

			// Update max delay
			if i.delay > maxDelay {
				maxDelay = i.delay
			}
		}
	}

	// Find the best step to have no lost frames
	for idx := 0; idx < h.d.stepsCount; idx++ {
		// Get step delay
		stepDelay := time.Duration(idx)*h.d.step + h.d.minimum

		// Update delay
		if idx == h.d.stepsCount-1 || stepDelay > maxDelay {
			if !h.disableDecrease || h.d.d < stepDelay {
				h.d.d = stepDelay
			}
			break
		}
	}
}

type ConstantDelayer struct {
	delay time.Duration
}

func NewConstantDelayer(delay time.Duration) *ConstantDelayer {
	return &ConstantDelayer{delay: delay}
}

func (d *ConstantDelayer) Apply(t time.Time) time.Time {
	return t.Add(-d.delay)
}

func (d *ConstantDelayer) Delay() time.Duration {
	return d.delay
}

func (d *ConstantDelayer) HandleFrame(delay time.Duration, n astiencoder.Node) {}
