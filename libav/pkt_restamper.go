package astilibav

import (
	"sync"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astikit"
)

// PktRestamper represents an object capable of restamping packets
type PktRestamper interface {
	Restamp(pkt *astiav.Packet)
}

type pktRestamperWithOffset struct {
	m       *sync.Mutex
	offsets map[int]int64
}

func newPktRestamperWithOffset() *pktRestamperWithOffset {
	return &pktRestamperWithOffset{
		m:       &sync.Mutex{},
		offsets: make(map[int]int64),
	}
}

func (r *pktRestamperWithOffset) restamp(pkt *astiav.Packet, fn func(dts int64) int64) {
	// Get timestamps
	dts := r.timestamp(pkt.Dts())
	pts := r.timestamp(pkt.Pts())

	// Compute offset
	r.m.Lock()
	offset, ok := r.offsets[pkt.StreamIndex()]
	if !ok {
		offset = fn(dts)
		r.offsets[pkt.StreamIndex()] = offset
	}
	r.m.Unlock()

	// Restamp
	delta := pts - dts
	dts += offset
	pkt.SetDts(dts)
	pkt.SetPts(dts + delta)
}

// We need to replace "no pts value" with a proper value otherwise it messes up the offset
// Right now it's replaced with 0 by default since in webrtc "no pts value" is
// equivalent to 0. But the best would be to let the caller decide which
// should be the best default value
func (r *pktRestamperWithOffset) timestamp(i int64) int64 {
	if i == astiav.NoPtsValue {
		return 0
	}
	return i
}

type pktRestamperStartFromZero struct {
	*pktRestamperWithOffset
}

// NewPktRestamperStartFromZero creates a new pkt restamper that starts timestamps from 0
func NewPktRestamperStartFromZero() PktRestamper {
	return &pktRestamperStartFromZero{pktRestamperWithOffset: newPktRestamperWithOffset()}
}

// Restamp implements the Restamper interface
func (r *pktRestamperStartFromZero) Restamp(pkt *astiav.Packet) {
	r.restamp(pkt, func(dts int64) int64 {
		return -dts
	})
}

type pktRestamperWithTime struct {
	fillGaps      bool
	firstAt       *time.Time
	frameDuration int64
	lastDTS       *int64
	timeBase      astiav.Rational
}

// NewPktRestamperWithTime creates a new pkt restamper that computes timestamps based on the time
// at which restamping was requested
// "fillGaps" option allows to:
//   - assign the current pkt to the previous DTS if previous DTS was never assigned
//   - assign the current pkt to the next DTS if current DTS is the same as previous DTS
// "frameDuration" must be a duration in frame time base
func NewPktRestamperWithTime(fillGaps bool, frameDuration int64, timeBase astiav.Rational) PktRestamper {
	return &pktRestamperWithTime{
		fillGaps:      fillGaps,
		frameDuration: frameDuration,
		timeBase:      timeBase,
	}
}

var now = time.Now

func (r *pktRestamperWithTime) Restamp(pkt *astiav.Packet) {
	n := now()
	if r.firstAt == nil {
		r.firstAt = astikit.TimePtr(n)
	}
	currentDTS := astiav.RescaleQ(int64(n.Sub(*r.firstAt)), NanosecondRational, r.timeBase)
	dts := currentDTS
	if r.fillGaps && r.lastDTS != nil {
		if previousDTS := currentDTS - r.frameDuration; *r.lastDTS < previousDTS {
			dts = previousDTS
		} else if nextDTS := currentDTS + r.frameDuration; *r.lastDTS == currentDTS {
			dts = nextDTS
		}
	}
	pkt.SetDts(dts)
	pkt.SetPts(dts)
	r.lastDTS = astikit.Int64Ptr(dts)
}
