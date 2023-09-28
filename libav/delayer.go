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
	LookBehind      time.Duration
	MarginGoingDown time.Duration
	MarginGoingUp   time.Duration
	Minimum         time.Duration
	Step            time.Duration
	StepsCount      uint
}

type AdaptiveDelayer struct {
	b               *adaptiveDelayerBuffer
	d               time.Duration
	lookBehind      time.Duration
	m               *sync.Mutex
	marginGoingDown time.Duration
	marginGoingUp   time.Duration
	minimum         time.Duration
	step            time.Duration
	stepsCount      int
}

type adaptiveDelayerBuffer struct {
	delays  map[astiencoder.Node][]time.Duration
	firstAt time.Time
}

func newAdaptiveDelayerBuffer(n time.Time) *adaptiveDelayerBuffer {
	return &adaptiveDelayerBuffer{
		delays:  make(map[astiencoder.Node][]time.Duration),
		firstAt: n,
	}
}

func NewAdaptiveDelayer(o AdaptiveDelayerOptions) *AdaptiveDelayer {
	d := &AdaptiveDelayer{
		d:               o.Minimum,
		lookBehind:      o.LookBehind,
		m:               &sync.Mutex{},
		marginGoingDown: o.MarginGoingDown,
		marginGoingUp:   o.MarginGoingUp,
		minimum:         o.Minimum,
		step:            o.Step,
		stepsCount:      int(o.StepsCount),
	}
	if d.stepsCount < 1 {
		d.stepsCount = 1
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
	// Nothing to do
	if d.b == nil || time.Since(d.b.firstAt) < d.lookBehind {
		return
	}

	// Get max average delay
	var maxAverageDelay *time.Duration
	for _, delays := range d.b.delays {
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
		for idx := 0; idx < d.stepsCount; idx++ {
			// Get step delay
			stepDelay := time.Duration(idx)*d.step + d.minimum

			// Same as current delay
			if d.d == stepDelay {
				continue
			}

			// Get boundaries
			var min, max time.Duration
			if stepDelay < d.d {
				min = stepDelay - d.step - d.marginGoingDown
				max = stepDelay - d.marginGoingDown
			} else {
				min = stepDelay - d.step - d.marginGoingUp
				max = stepDelay - d.marginGoingUp
			}

			// Update delay
			if (idx == 0 || *maxAverageDelay >= min) && (idx == d.stepsCount-1 || *maxAverageDelay <= max) {
				d.d = stepDelay
				break
			}
		}
	}

	// Reset buffer
	d.b = nil
}

func (d *AdaptiveDelayer) HandleFrame(delay time.Duration, n astiencoder.Node) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Make sure to update delay first
	d.updateDelayUnsafe()

	// Make sure to create buffer
	if d.b == nil {
		d.b = newAdaptiveDelayerBuffer(now())
	}

	// Add delay
	d.b.delays[n] = append(d.b.delays[n], delay)
}

func (d *AdaptiveDelayer) SetMinimum(min time.Duration) {
	d.minimum = min
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
