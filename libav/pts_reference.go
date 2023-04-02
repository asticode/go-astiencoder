package astilibav

import (
	"sync"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
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
	return astiav.RescaleQ(int64(t.Sub(r.t))+r.pts, nanosecondRational, timeBase)
}

func (r PTSReference) PTSFromTime(t time.Time, timeBase astiav.Rational) int64 {
	r.m.Lock()
	defer r.m.Unlock()
	return r.ptsFromTimeUnsafe(t, timeBase)
}

func (r PTSReference) timeFromPTSUnsafe(pts int64, timeBase astiav.Rational) time.Time {
	return r.t.Add(time.Duration(astiav.RescaleQ(pts, timeBase, nanosecondRational) - r.pts))
}

func (r PTSReference) TimeFromPTS(pts int64, timeBase astiav.Rational) time.Time {
	r.m.Lock()
	defer r.m.Unlock()
	return r.timeFromPTSUnsafe(pts, timeBase)
}

func (r *PTSReference) updateUnsafe(pts int64, t time.Time, timeBase astiav.Rational) *PTSReference {
	r.pts = astiav.RescaleQ(pts, timeBase, nanosecondRational)
	r.t = t
	return r
}

func (r *PTSReference) Update(pts int64, t time.Time, timeBase astiav.Rational) *PTSReference {
	r.m.Lock()
	defer r.m.Unlock()
	return r.updateUnsafe(pts, t, timeBase)
}

type ptsReferences struct {
	m *sync.Mutex
	p map[astiencoder.Node]*PTSReference
}

func newPTSReferences(p map[astiencoder.Node]*PTSReference) *ptsReferences {
	// Create pts references
	r := &ptsReferences{
		m: &sync.Mutex{},
		p: make(map[astiencoder.Node]*PTSReference),
	}

	// Copy pts references
	for k, v := range p {
		r.p[k] = v
	}
	return r
}

func (rs *ptsReferences) get(n astiencoder.Node) *PTSReference {
	rs.m.Lock()
	defer rs.m.Unlock()
	return rs.p[n]
}

func (rs *ptsReferences) set(n astiencoder.Node, r *PTSReference) {
	rs.m.Lock()
	defer rs.m.Unlock()
	rs.p[n] = r
}

func (rs *ptsReferences) lockAll() {
	rs.m.Lock()
	defer rs.m.Unlock()
	for _, r := range rs.p {
		r.lock()
	}
}

func (rs *ptsReferences) unlockAll() {
	rs.m.Lock()
	defer rs.m.Unlock()
	for _, r := range rs.p {
		r.unlock()
	}
}
