package astilibav

import (
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avutil"
)

// FrameRestamper represents an object capable of restamping frames
type FrameRestamper interface {
	Restamp(f *avutil.Frame)
}

type frameRestamperWithValue struct {
	lastValue *int64
}

func newFrameRestamperWithValue() *frameRestamperWithValue {
	return &frameRestamperWithValue{}
}

func (r *frameRestamperWithValue) restamp(f *avutil.Frame, fn func(v *int64) *int64) {
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
}

// NewFrameRestamperWithFrameDuration creates a new frame restamper that starts timestamps from 0 and increments them
// of frameDuration
// frameDuration must be a duration in frame time base
func NewFrameRestamperWithFrameDuration(frameDuration int64) FrameRestamper {
	return &frameRestamperWithFrameDuration{
		frameRestamperWithValue: newFrameRestamperWithValue(),
		frameDuration:           frameDuration,
	}
}

// Restamp implements the FrameRestamper interface
func (r *frameRestamperWithFrameDuration) Restamp(f *avutil.Frame) {
	r.restamp(f, func(v *int64) *int64 {
		if v != nil {
			return astikit.Int64Ptr(*v + r.frameDuration)
		}
		return astikit.Int64Ptr(0)
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
func (r *frameRestamperWithModulo) Restamp(f *avutil.Frame) {
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
