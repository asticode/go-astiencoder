package astilibav

import (
	"sync"

	"github.com/asticode/go-astitools/ptr"
	"github.com/asticode/goav/avcodec"
)

// PktRestamper represents an object capable of restamping packets
type PktRestamper interface {
	Restamp(pkt *avcodec.Packet)
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

func (r *pktRestamperWithOffset) restamp(pkt *avcodec.Packet, fn func(pkt *avcodec.Packet) int64) {
	// Compute offset
	r.m.Lock()
	offset, ok := r.offsets[pkt.StreamIndex()]
	if !ok {
		offset = fn(pkt)
		r.offsets[pkt.StreamIndex()] = offset
	}
	r.m.Unlock()

	// Restamp
	delta := pkt.Pts() - pkt.Dts()
	dts := pkt.Dts() + offset
	pkt.SetDts(dts)
	pkt.SetPts(dts + delta)
}

type pktRestamperStartFromZero struct {
	*pktRestamperWithOffset
}

// NewPktRestamperStartFromZero creates a new pkt restamper that starts timestamps from 0
func NewPktRestamperStartFromZero() PktRestamper {
	return &pktRestamperStartFromZero{pktRestamperWithOffset: newPktRestamperWithOffset()}
}

// Restamp implements the Restamper interface
func (r *pktRestamperStartFromZero) Restamp(pkt *avcodec.Packet) {
	r.restamp(pkt, func(pkt *avcodec.Packet) int64 {
		return -pkt.Dts()
	})
}

type pktRestamperWithValue struct {
	m      *sync.Mutex
	values map[int]*int64
}

func newPktRestamperWithValue() *pktRestamperWithValue {
	return &pktRestamperWithValue{
		m:      &sync.Mutex{},
		values: make(map[int]*int64),
	}
}

func (r *pktRestamperWithValue) restamp(pkt *avcodec.Packet, fn func(v *int64) *int64) {
	// Get last value
	r.m.Lock()
	value := r.values[pkt.StreamIndex()]
	r.m.Unlock()

	// Compute new value
	value = fn(value)
	r.values[pkt.StreamIndex()] = value

	// Restamp
	delta := pkt.Pts() - pkt.Dts()
	pkt.SetDts(*value)
	pkt.SetPts(*value + delta)
}

type pktRestamperWithPktDuration struct {
	*pktRestamperWithValue
}

// NewPktRestamperWithPktDuration creates a new pkt restamper that starts timestamps from 0 and increments them
// of pkt.Duration()
func NewPktRestamperWithPktDuration() PktRestamper {
	return &pktRestamperWithPktDuration{pktRestamperWithValue: newPktRestamperWithValue()}
}

// Restamp implements the FrameRestamper interface
func (r *pktRestamperWithPktDuration) Restamp(pkt *avcodec.Packet) {
	r.restamp(pkt, func(v *int64) *int64 {
		if v != nil {
			return astiptr.Int64(*v + pkt.Duration())
		}
		return astiptr.Int64(0)
	})
}
