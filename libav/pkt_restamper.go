package astilibav

import (
	"sync"

	"github.com/asticode/go-astiav"
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

type PktRestamperStartFromZero struct {
	*pktRestamperWithOffset
}

// NewPktRestamperStartFromZero creates a new pkt restamper that starts timestamps from 0
func NewPktRestamperStartFromZero() *PktRestamperStartFromZero {
	return &PktRestamperStartFromZero{pktRestamperWithOffset: newPktRestamperWithOffset()}
}

// Restamp implements the Restamper interface
func (r *PktRestamperStartFromZero) Restamp(pkt *astiav.Packet) {
	r.restamp(pkt, func(dts int64) int64 {
		return -dts
	})
}
