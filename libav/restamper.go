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
	// Updates the restamper offsets
	// duration must be a computation of timestamps FIRST multiplied by 1e9 THEN rescaled to a 1/1 timebase
	UpdateOffsets(duration int64)
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
func (r *restamperBase) UpdateOffsets(delta int64) {
	r.m.Lock()
	defer r.m.Unlock()
	for k := range r.offsets {
		r.offsets[k] += delta
	}
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
	dts := pkt.Dts() + int64(avutil.AvRescaleQ(offset, defaultRational, d.TimeBase())/1e9)
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
		return int64(r.firstPacketAt.Sub(r.startAt)) - avutil.AvRescaleQ(pkt.Dts()*1e9, d.TimeBase(), defaultRational)
	})
}

// Time implements the Restamper interface
func (r *restamperLive) Time(ts int64, d Descriptor) time.Time {
	return r.startAt.Add(time.Duration(avutil.AvRescaleQ(ts*1e9, d.TimeBase(), defaultRational)))
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
		return -avutil.AvRescaleQ(pkt.Dts()*1e9, d.TimeBase(), defaultRational)
	})
}

// Time implements the Restamper interface
func (r *restamperZero) Time(ts int64, d Descriptor) time.Time {
	return r.firstPacketAt.Add(time.Duration(avutil.AvRescaleQ(ts*1e9, d.TimeBase(), defaultRational)))
}
