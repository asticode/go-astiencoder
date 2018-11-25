package astilibav

import (
	"time"

	"github.com/asticode/go-astitools/ptr"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
)

// Restamper represents an object capable of restamping packets
type Restamper interface {
	// Returns the time.Time of a restamped pkt ts
	Time(ts int64, d Descriptor) time.Time
	Restamp(pkt *avcodec.Packet, d Descriptor)
	// Updates the restamper offset
	// duration must be a computation of timestamps FIRST multiplied by 1e9 THEN rescaled to a 1/1 timebase
	UpdateOffset(duration int64)
}

// NewRestamperLive creates a new live restamper
func NewRestamperLive(startAt time.Time) Restamper {
	return &restamperLive{startAt: startAt}
}

type restamperLive struct {
	firstPacketAt time.Time
	offset        *int64
	startAt       time.Time
}

// UpdateOffset implements the Restamper interface
func (r *restamperLive) UpdateOffset(delta int64) {
	if r.offset != nil {
		*r.offset += delta
	}
}

// Restamp implements the Restamper interface
func (r *restamperLive) Restamp(pkt *avcodec.Packet, d Descriptor) {
	// Compute offset
	if r.offset == nil {
		r.offset = astiptr.Int64(int64(r.firstPacketAt.Sub(r.startAt)) - avutil.AvRescaleQ(pkt.Dts()*1e9, d.TimeBase(), defaultRational))
	}

	// Set dts and pts
	delta := pkt.Pts() - pkt.Dts()
	dts := pkt.Dts() + int64(avutil.AvRescaleQ(*r.offset, defaultRational, d.TimeBase())/1e9)
	pkt.SetDts(dts)
	pkt.SetPts(dts + delta)
}

// Time implements the Restamper interface
func (r *restamperLive) Time(ts int64, d Descriptor) time.Time {
	return r.startAt.Add(time.Duration(avutil.AvRescaleQ(ts*1e9, d.TimeBase(), defaultRational)))
}
