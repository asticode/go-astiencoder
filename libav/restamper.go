package astilibav

import (
	"sync"
	"time"

	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
)

// Restamper represents an object capable of restamping packets
type Restamper interface {
	FirstPkt()
	Restamp(pkt *avcodec.Packet, d Descriptor)
	// Returns the time.Time of a restamped pkt ts
	Time(ts int64, d Descriptor) time.Time
	// Updates restamper stream offset with a duration in stream.time_base
	UpdateOffset(duration int64, streamIdx int)
}

type restamperBase struct {
	firstPacketAt time.Time
	m             *sync.Mutex
	offsets       map[int]int64
}

func newRestamperBase() *restamperBase {
	return &restamperBase{
		m:       &sync.Mutex{},
		offsets: make(map[int]int64),
	}
}

// FirstPkt implements the Restamper interface
func (r *restamperBase) FirstPkt() {
	r.firstPacketAt = time.Now()
}

// UpdateOffset implements the Restamper interface
func (r *restamperBase) UpdateOffset(delta int64, streamIdx int) {
	r.m.Lock()
	defer r.m.Unlock()
	offset , ok := r.offsets[streamIdx]
	if !ok {
		return
	}
	r.offsets[streamIdx] = offset + delta
}

func (r *restamperBase) restamp(pkt *avcodec.Packet, d Descriptor, fn func(pkt *avcodec.Packet, d Descriptor) int64) {
	// Compute offset
	r.m.Lock()
	offset, ok := r.offsets[pkt.StreamIndex()]
	if !ok {
		offset = fn(pkt, d)
		r.offsets[pkt.StreamIndex()] = offset
	}
	r.m.Unlock()

	// Restamp
	delta := pkt.Pts() - pkt.Dts()
	dts := pkt.Dts() + offset
	pkt.SetDts(dts)
	pkt.SetPts(dts + delta)
}

type restamperLive struct {
	*restamperBase
	startAt time.Time
}

// NewRestamperLive creates a new live restamper
func NewRestamperLive(startAt time.Time) Restamper {
	return &restamperLive{
		restamperBase: newRestamperBase(),
		startAt:       startAt,
	}
}

// Restamp implements the Restamper interface
func (r *restamperLive) Restamp(pkt *avcodec.Packet, d Descriptor) {
	r.restamp(pkt, d, func(pkt *avcodec.Packet, d Descriptor) int64 {
		return avutil.AvRescaleQ(int64(r.firstPacketAt.Sub(r.startAt)), nanosecondRational, d.TimeBase()) - pkt.Dts()
	})
}

// Time implements the Restamper interface
func (r *restamperLive) Time(ts int64, d Descriptor) time.Time {
	return r.startAt.Add(time.Duration(avutil.AvRescaleQ(ts, d.TimeBase(), nanosecondRational)))
}

type restamperZero struct {
	*restamperBase
}

// NewRestamperZero creates a new zero restamper
func NewRestamperZero() Restamper {
	return &restamperZero{restamperBase: newRestamperBase()}
}

// Restamp implements the Restamper interface
func (r *restamperZero) Restamp(pkt *avcodec.Packet, d Descriptor) {
	r.restamp(pkt, d, func(pkt *avcodec.Packet, d Descriptor) int64 {
		return -pkt.Dts()
	})
}

// Time implements the Restamper interface
func (r *restamperZero) Time(ts int64, d Descriptor) time.Time {
	return r.firstPacketAt.Add(time.Duration(avutil.AvRescaleQ(ts, d.TimeBase(), nanosecondRational)))
}
