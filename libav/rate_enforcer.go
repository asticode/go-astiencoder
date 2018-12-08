package astilibav

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/time"
	"github.com/asticode/goav/avutil"
)

// RateEnforcer represents an object that can enforce rate
type RateEnforcer interface {
	Add(f *avutil.Frame, d Descriptor)
	Close()
	ReferencePtsChanged()
	Start(ctx context.Context, fn func(f *avutil.Frame, d Descriptor))
}

// RateEnforcerDefault represents the default rate enforcer
type RateEnforcerDefault struct {
	buf             []*rateEnforcerItem
	c               *astiencoder.Closer
	e               *astiencoder.EventEmitter
	m               *sync.Mutex
	p               *framePool
	period          time.Duration
	previousItem    *rateEnforcerItem
	referencePtsIdx int
	slotsCount      int
	slots           []*rateEnforcerSlot
	timeBase        avutil.Rational
}

type rateEnforcerSlot struct {
	d               Descriptor
	i               *rateEnforcerItem
	referencePtsIdx int
	ptsMax          int64
	ptsMin          int64
}

type rateEnforcerItem struct {
	d               Descriptor
	f               *avutil.Frame
	referencePtsIdx int
}

// NewRateEnforcerDefault creates a new default rate enforcer
func NewRateEnforcerDefault(frameRate avutil.Rational, framesDelay int, e *astiencoder.EventEmitter) (r *RateEnforcerDefault) {
	r = &RateEnforcerDefault{
		c:          astiencoder.NewCloser(),
		e:          e,
		m:          &sync.Mutex{},
		period:     time.Duration(float64(1e9) / frameRate.ToDouble()),
		slotsCount: int(math.Max(float64(framesDelay), 1)),
		slots:      []*rateEnforcerSlot{nil},
		timeBase:   avutil.NewRational(frameRate.Den(), frameRate.Num()),
	}
	r.p = newFramePool(r.c)
	return
}

// Close implements the RateEnforcer interface
func (r *RateEnforcerDefault) Close() {
	r.c.Close()
}

// ReferencePtsChanged implements the RateEnforcer interface
func (r *RateEnforcerDefault) ReferencePtsChanged() {
	r.m.Lock()
	defer r.m.Unlock()
	r.referencePtsIdx++
}

func (r *RateEnforcerDefault) nextPts(pts int64, timeBase avutil.Rational) int64 {
	return pts + int64(r.timeBase.ToDouble()/timeBase.ToDouble())
}

func (r *RateEnforcerDefault) newRateEnforcerSlot(pts int64, d Descriptor, referencePtsIdx int) *rateEnforcerSlot {
	return &rateEnforcerSlot{
		d:               d,
		referencePtsIdx: referencePtsIdx,
		ptsMax:          r.nextPts(pts, d.TimeBase()),
		ptsMin:          pts,
	}
}

func (r *RateEnforcerDefault) newRateEnforcerItem(d Descriptor) *rateEnforcerItem {
	return &rateEnforcerItem{
		d:               d,
		referencePtsIdx: r.referencePtsIdx,
		f:               r.p.get(),
	}
}

// Add implements the RateEnforcer interface
func (r *RateEnforcerDefault) Add(f *avutil.Frame, d Descriptor) {
	// Lock
	r.m.Lock()
	defer r.m.Unlock()

	// Stream changed
	if r.slots[len(r.slots)-1] == nil || r.referencePtsIdx > r.slots[len(r.slots)-1].referencePtsIdx {
		r.slots[len(r.slots)-1] = r.newRateEnforcerSlot(f.Pts(), d, r.referencePtsIdx)
	}

	// Create item
	i := r.newRateEnforcerItem(d)

	// Copy frame
	if ret := avutil.AvFrameRef(i.f, f); ret < 0 {
		emitAvError(r.e, ret, "avutil.AvFrameRef failed")
		return
	}

	// Append item
	r.buf = append(r.buf, i)
}

// Start implements the RateEnforcer interface
func (r *RateEnforcerDefault) Start(ctx context.Context, fn func(f *avutil.Frame, d Descriptor)) {
	go func() {
		nextAt := time.Now()
		for {
			func() {
				// Compute next at
				nextAt = nextAt.Add(r.period)

				// Sleep until next at
				if delta := nextAt.Sub(time.Now()); delta > 0 {
					astitime.Sleep(ctx, delta)
				}

				// Check context
				if ctx.Err() != nil {
					return
				}

				// Lock
				r.m.Lock()
				defer r.m.Unlock()

				// Make sure to add next slot
				defer func() {
					var s *rateEnforcerSlot
					if ps := r.slots[len(r.slots)-1]; ps != nil {
						s = r.newRateEnforcerSlot(ps.ptsMax, ps.d, ps.referencePtsIdx)
					}
					r.slots = append(r.slots, s)
				}()

				// Not enough slots
				if len(r.slots) < r.slotsCount {
					return
				}

				// Distribute
				r.distribute()

				// Dispatch
				i, previous := r.current()
				if i != nil {
					fn(i.f, i.d)
				}

				// Remove first slot
				if !previous {
					r.p.put(i.f)
				}
				r.slots = r.slots[1:]
			}()
		}
	}()
}

func (r *RateEnforcerDefault) distribute() {
	// Loop through slots
	for _, s := range r.slots {
		// Slot is empty or already has an item
		if s == nil || s.i != nil {
			continue
		}

		// Loop through buffer
		for idx := 0; idx < len(r.buf); idx++ {
			// Not the same stream idx
			if r.buf[idx].referencePtsIdx != s.referencePtsIdx {
				if r.buf[idx].referencePtsIdx < s.referencePtsIdx {
					r.buf = append(r.buf[:idx], r.buf[idx+1:]...)
				}
				continue
			}

			// Add to slot or remove if pts is older
			if s.ptsMin <= r.buf[idx].f.Pts() && s.ptsMax > r.buf[idx].f.Pts() {
				s.i = r.buf[idx]
				r.buf = append(r.buf[:idx], r.buf[idx+1:]...)
				continue
			} else if s.ptsMin > r.buf[idx].f.Pts() {
				r.p.put(r.buf[idx].f)
				r.buf = append(r.buf[:idx], r.buf[idx+1:]...)
				continue
			}
		}
	}
}

func (r *RateEnforcerDefault) current() (i *rateEnforcerItem, previous bool) {
	if r.slots[0] != nil && r.slots[0].i != nil {
		// Get item
		i = r.slots[0].i

		// Create previous item
		if r.previousItem == nil {
			// Create item
			r.previousItem = &rateEnforcerItem{
				d: i.d,
				f: r.p.get(),
			}
		} else {
			avutil.AvFrameUnref(r.previousItem.f)
		}

		// Copy frame
		if ret := avutil.AvFrameRef(r.previousItem.f, i.f); ret < 0 {
			emitAvError(r.e, ret, "avutil.AvFrameRef failed")
			r.p.put(r.previousItem.f)
			r.previousItem = nil
		}
	} else {
		i = r.previousItem
		previous = true
	}
	return
}
