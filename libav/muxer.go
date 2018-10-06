package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/asticode/goav/avutil"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/pkg/errors"
)

var countMuxer uint64

// Muxer represents an object capable of muxing packets into an output
type Muxer struct {
	*astiencoder.BaseNode
	c                *astiencoder.Closer
	ctxFormat        *avformat.Context
	e                astiencoder.EmitEventFunc
	o                *sync.Once
	q                *astisync.CtxQueue
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// NewMuxer creates a new muxer
func NewMuxer(ctxFormat *avformat.Context, e astiencoder.EmitEventFunc, c *astiencoder.Closer) (m *Muxer) {
	count := atomic.AddUint64(&countMuxer, uint64(1))
	m = &Muxer{
		BaseNode: astiencoder.NewBaseNode(e, astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Muxes to %s", ctxFormat.Filename()),
			Label:       fmt.Sprintf("Muxer #%d", count),
			Name:        fmt.Sprintf("muxer_%d", count),
		}),
		c:                c,
		ctxFormat:        ctxFormat,
		e:                e,
		o:                &sync.Once{},
		q:                astisync.NewCtxQueue(),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	m.addStats()
	return
}

func (m *Muxer) addStats() {
	// Add incoming rate
	m.Stater().AddStat(astistat.StatMetadata{
		Description: "Number of packets coming in the muxer per second",
		Label:       "Incoming rate",
		Unit:        "pps",
	}, m.statIncomingRate)

	// Add work ratio
	m.Stater().AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, m.statWorkRatio)

	// Add queue stats
	m.q.AddStats(m.Stater())
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	m.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go m.q.HandleCtx(m.Context())

		// Make sure to write header once
		var ret int
		m.o.Do(func() { ret = m.ctxFormat.AvformatWriteHeader(nil) })
		if ret < 0 {
			emitAvError(m.e, ret, "m.ctxFormat.AvformatWriteHeader on %s failed", m.ctxFormat.Filename())
			return
		}

		// Write trailer once everything is done
		m.c.Add(func() error {
			if ret := m.ctxFormat.AvWriteTrailer(); ret < 0 {
				return errors.Wrapf(newAvError(ret), "m.ctxFormat.AvWriteTrailer on %s failed", m.ctxFormat.Filename())
			}
			return nil
		})

		// Make sure to stop the queue properly
		defer m.q.Stop()

		// Start queue
		m.q.Start(func(p interface{}) {
			// Handle pause
			defer m.HandlePause()

			// Assert payload
			pkt := p.(*avcodec.Packet)

			// Increment incoming rate
			m.statIncomingRate.Add(1)

			// Write frame
			m.statWorkRatio.Add(true)
			if ret := m.ctxFormat.AvInterleavedWriteFrame((*avformat.Packet)(unsafe.Pointer(pkt))); ret < 0 {
				m.statWorkRatio.Done(true)
				emitAvError(m.e, ret, "m.ctxFormat.AvInterleavedWriteFrame on %+v failed", pkt)
				return
			}
			m.statWorkRatio.Done(true)
		})
	})
}

// MuxerPktHandler is an object that can handle a pkt for the muxer
type MuxerPktHandler struct {
	*Muxer
	o *avformat.Stream
	t TimeBaser
}

// TimeBaser is an object that can return a time base
type TimeBaser interface {
	TimeBase() avutil.Rational
}

// NewHandler creates
func (m *Muxer) NewPktHandler(o *avformat.Stream, t TimeBaser) *MuxerPktHandler {
	return &MuxerPktHandler{
		Muxer: m,
		o:     o,
		t:     t,
	}
}

// HandlePkt implements the PktHandler interface
func (h *MuxerPktHandler) HandlePkt(pkt *avcodec.Packet) {
	// Rescale timestamps
	pkt.AvPacketRescaleTs(h.t.TimeBase(), h.o.TimeBase())

	// Set stream index
	pkt.SetStreamIndex(h.o.Index())

	// Send pkt
	h.q.Send(pkt, true)
}
