package astilibav

import (
	"sync"

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

type PktRestamperStartFromZero struct {
	*pktRestamperWithOffset
}

// NewPktRestamperStartFromZero creates a new pkt restamper that starts timestamps from 0
func NewPktRestamperStartFromZero() *PktRestamperStartFromZero {
	return &PktRestamperStartFromZero{pktRestamperWithOffset: newPktRestamperWithOffset()}
}

// Restamp implements the Restamper interface
func (r *PktRestamperStartFromZero) Restamp(pkt *avcodec.Packet) {
	r.restamp(pkt, func(pkt *avcodec.Packet) int64 {
		return -pkt.Dts()
	})
}

type PktRestamperWithPktDuration struct {
	lastItem map[int]*pktRestamperWithPktDurationItem
	m        *sync.Mutex
}

type pktRestamperWithPktDurationItem struct {
	dts      int64
	duration int64
}

// NewPktRestamperWithPktDuration creates a new pkt restamper that starts timestamps from 0 and increments them
// of the previous pkt.Duration()
func NewPktRestamperWithPktDuration() *PktRestamperWithPktDuration {
	return &PktRestamperWithPktDuration{
		lastItem: make(map[int]*pktRestamperWithPktDurationItem),
		m:        &sync.Mutex{},
	}
}

// Add adds delta to the last item duration of the specified stream index
func (r *PktRestamperWithPktDuration) Add(delta int64, idx int) {
	r.m.Lock()
	defer r.m.Unlock()
	if i, ok := r.lastItem[idx]; ok && i != nil {
		i.duration += delta
	}
}

// Restamp implements the FrameRestamper interface
func (r *PktRestamperWithPktDuration) Restamp(pkt *avcodec.Packet) {
	// Get last item
	r.m.Lock()
	lastItem := r.lastItem[pkt.StreamIndex()]
	r.m.Unlock()

	// Compute new item
	item := &pktRestamperWithPktDurationItem{
		duration: pkt.Duration(),
	}
	if lastItem != nil {
		item.dts = lastItem.dts + lastItem.duration
	}

	// Set new item
	r.m.Lock()
	r.lastItem[pkt.StreamIndex()] = item
	r.m.Unlock()

	// Restamp
	delta := pkt.Pts() - pkt.Dts()
	pkt.SetDts(item.dts)
	pkt.SetPts(item.dts + delta)
}
