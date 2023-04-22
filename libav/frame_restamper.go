package astilibav

import (
	"sync"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

// FrameRestamper represents an object capable of restamping frames
type FrameRestamper interface {
	Restamp(f *astiav.Frame)
}

type frameRestamperWithValue struct {
	lastValue *int64
}

func newFrameRestamperWithValue() *frameRestamperWithValue {
	return &frameRestamperWithValue{}
}

func (r *frameRestamperWithValue) restamp(f *astiav.Frame, fn func(v *int64) *int64) {
	// Compute new value
	v := fn(r.lastValue)

	// Restamp
	f.SetPts(*v)

	// Store new value
	r.lastValue = v
}

type frameRestamperWithFrameDuration struct {
	*frameRestamperWithValue
	frameDuration int64
	startPTSFunc  FrameRestamperStartPTSFunc
}

type FrameRestamperStartPTSFunc func() int64

func FrameRestamperStartPTSFuncFromPTSReference(r *PTSReference, timeBase astiav.Rational) FrameRestamperStartPTSFunc {
	return func() int64 { return r.PTSFromTime(time.Now(), timeBase) }
}

// NewFrameRestamperWithFrameDuration creates a new frame restamper that starts timestamps from startPTS and increments them
// of frameDuration
// frameDuration must be a duration in frame time base
func NewFrameRestamperWithFrameDuration(frameDuration int64, startPTSFunc FrameRestamperStartPTSFunc) FrameRestamper {
	return &frameRestamperWithFrameDuration{
		frameRestamperWithValue: newFrameRestamperWithValue(),
		frameDuration:           frameDuration,
		startPTSFunc:            startPTSFunc,
	}
}

// Restamp implements the FrameRestamper interface
func (r *frameRestamperWithFrameDuration) Restamp(f *astiav.Frame) {
	r.restamp(f, func(v *int64) *int64 {
		if v != nil {
			return astikit.Int64Ptr(*v + r.frameDuration)
		}
		var nv int64
		if r.startPTSFunc != nil {
			nv = r.startPTSFunc()
		}
		return astikit.Int64Ptr(nv)
	})
}

type frameRestamperWithModulo struct {
	*frameRestamperWithValue
	frameDuration int64
	lastRealValue int64
}

// NewFrameRestamperWithModulo creates a new frame restamper that makes sure that PTS % frame duration = 0
// frameDuration must be a duration in frame time base
func NewFrameRestamperWithModulo(frameDuration int64) FrameRestamper {
	return &frameRestamperWithModulo{
		frameRestamperWithValue: newFrameRestamperWithValue(),
		frameDuration:           frameDuration,
	}
}

// Restamp implements the FrameRestamper interface
func (r *frameRestamperWithModulo) Restamp(f *astiav.Frame) {
	r.restamp(f, func(v *int64) *int64 {
		defer func() { r.lastRealValue = f.Pts() }()
		if v != nil {
			nv := astikit.Int64Ptr(f.Pts() - (f.Pts() % r.frameDuration))
			if *nv <= *v {
				nv = astikit.Int64Ptr(*v + r.frameDuration)
			}
			return nv
		}
		return astikit.Int64Ptr(f.Pts() - (f.Pts() % r.frameDuration))
	})
}

type FrameRestamperOffseter interface {
	Offset(f *astiav.Frame, frameDuration int64, timeBase astiav.Rational) int64
}

type FrameRestamperOffseterWithStartFromZero struct {
	lastFrameDuration int64  // In nanosecond rational
	lastPTS           *int64 // In nanosecond rational
	m                 *sync.Mutex
	offset            *int64 // In nanosecond rational
	updateOnNextFrame bool
}

func NewFrameRestamperOffseterWithStartFromZero() *FrameRestamperOffseterWithStartFromZero {
	return &FrameRestamperOffseterWithStartFromZero{m: &sync.Mutex{}}
}

func (o *FrameRestamperOffseterWithStartFromZero) Offset(f *astiav.Frame, frameDuration int64, timeBase astiav.Rational) int64 {
	// Lock
	o.m.Lock()
	defer o.m.Unlock()

	// Get pts
	pts := astiav.RescaleQ(f.Pts(), timeBase, NanosecondRational)

	// Get offset
	var offset int64
	if o.offset != nil {
		offset = *o.offset
	} else {
		offset = pts
	}

	// Update on next frame
	if o.updateOnNextFrame && o.lastPTS != nil {
		offset += pts - *o.lastPTS - o.lastFrameDuration
		o.updateOnNextFrame = false
	}

	// Store offset
	o.offset = &offset

	// Store last pts
	if o.lastPTS == nil || pts > *o.lastPTS {
		o.lastPTS = astikit.Int64Ptr(pts)
		o.lastFrameDuration = astiav.RescaleQ(frameDuration, timeBase, NanosecondRational)
	}

	// Rescale
	return astiav.RescaleQ(-offset, NanosecondRational, timeBase)
}

func (o *FrameRestamperOffseterWithStartFromZero) Paused() {
	o.m.Lock()
	defer o.m.Unlock()
	o.updateOnNextFrame = true
}

type frameRestamperWithOffset struct {
	o             FrameRestamperOffseter
	frameDuration int64
	timeBase      astiav.Rational
}

func NewFrameRestamperWithOffset(o FrameRestamperOffseter, frameDuration int64, timeBase astiav.Rational) FrameRestamper {
	return &frameRestamperWithOffset{
		o:             o,
		frameDuration: frameDuration,
		timeBase:      timeBase,
	}
}

func (r *frameRestamperWithOffset) Restamp(f *astiav.Frame) {
	// Restamp
	f.SetPts(f.Pts() + r.o.Offset(f, r.frameDuration, r.timeBase))
}
