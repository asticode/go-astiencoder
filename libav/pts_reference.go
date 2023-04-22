package astilibav

import (
	"sync"
	"time"

	"github.com/asticode/go-astiav"
)

type PTSReference struct {
	m *sync.Mutex
	// We use a PTS in nanosecond timebase so that when sharing a PTS reference between streams with different
	// timebase there's no RescaleQ rounding issues
	pts int64
	t   time.Time
}

func NewPTSReference() *PTSReference {
	return &PTSReference{m: &sync.Mutex{}}
}

func (r PTSReference) lock() {
	r.m.Lock()
}

func (r PTSReference) unlock() {
	r.m.Unlock()
}

func (r PTSReference) isZeroUnsafe() bool {
	return r.t.IsZero()
}

func (r PTSReference) ptsFromTimeUnsafe(t time.Time, timeBase astiav.Rational) int64 {
	return astiav.RescaleQ(int64(t.Sub(r.t))+r.pts, NanosecondRational, timeBase)
}

func (r PTSReference) PTSFromTime(t time.Time, timeBase astiav.Rational) int64 {
	r.m.Lock()
	defer r.m.Unlock()
	return r.ptsFromTimeUnsafe(t, timeBase)
}

func (r PTSReference) timeFromPTSUnsafe(pts int64, timeBase astiav.Rational) time.Time {
	return r.t.Add(time.Duration(astiav.RescaleQ(pts, timeBase, NanosecondRational) - r.pts))
}

func (r PTSReference) TimeFromPTS(pts int64, timeBase astiav.Rational) time.Time {
	r.m.Lock()
	defer r.m.Unlock()
	return r.timeFromPTSUnsafe(pts, timeBase)
}

func (r *PTSReference) updateUnsafe(pts int64, t time.Time, timeBase astiav.Rational) *PTSReference {
	r.pts = astiav.RescaleQ(pts, timeBase, NanosecondRational)
	r.t = t
	return r
}

func (r *PTSReference) Update(pts int64, t time.Time, timeBase astiav.Rational) *PTSReference {
	r.m.Lock()
	defer r.m.Unlock()
	return r.updateUnsafe(pts, t, timeBase)
}
