package astilibav

import (
	"sync"

	"github.com/asticode/go-astitools/ptr"
	"github.com/asticode/goav/avutil"
)

// FrameRestamper represents an object capable of restamping frames
type FrameRestamper interface {
	Restamp(f *avutil.Frame, id interface{})
}

type frameRestamperWithValue struct {
	m      *sync.Mutex
	values map[interface{}]*int64
}

func newFrameRestamperWithValue() *frameRestamperWithValue {
	return &frameRestamperWithValue{
		m:      &sync.Mutex{},
		values: make(map[interface{}]*int64),
	}
}

func (r *frameRestamperWithValue) restamp(f *avutil.Frame, id interface{}, fn func(v *int64) *int64) {
	// Get last value
	r.m.Lock()
	value := r.values[id]
	r.m.Unlock()

	// Compute new value
	value = fn(value)
	r.values[id] = value

	// Restamp
	f.SetPts(*value)
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
func (r *frameRestamperWithFrameDuration) Restamp(f *avutil.Frame, id interface{}) {
	r.restamp(f, id, func(v *int64) *int64 {
		if v != nil {
			return astiptr.Int64(*v + r.frameDuration)
		}
		return astiptr.Int64(0)
	})
}
