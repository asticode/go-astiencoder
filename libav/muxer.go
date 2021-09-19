package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avformat"
)

var countMuxer uint64

// Muxer represents an object capable of muxing packets into an output
type Muxer struct {
	*astiencoder.BaseNode
	c                 *astikit.Chan
	cl                *astikit.Closer
	ctxFormat         *avformat.Context
	eh                *astiencoder.EventHandler
	o                 *sync.Once
	p                 *pktPool
	restamper         PktRestamper
	statIncomingRate  *astikit.CounterRateStat
	statOutgoingRate  *astikit.CounterRateStat
	statProcessedRate *astikit.CounterRateStat
}

// MuxerOptions represents muxer options
type MuxerOptions struct {
	Format     *avformat.OutputFormat
	FormatName string
	Node       astiencoder.NodeOptions
	Restamper  PktRestamper
	URL        string
}

// NewMuxer creates a new muxer
func NewMuxer(o MuxerOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (m *Muxer, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countMuxer, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("muxer_%d", count), fmt.Sprintf("Muxer #%d", count), fmt.Sprintf("Muxes to %s", o.URL), "muxer")

	// Create muxer
	m = &Muxer{
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		cl:                c.NewChild(),
		eh:                eh,
		o:                 &sync.Once{},
		restamper:         o.Restamper,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statOutgoingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	m.BaseNode = astiencoder.NewBaseNode(o.Node, eh, s, m, astiencoder.EventTypeToNodeEventName)

	// Create pkt pool
	m.p = newPktPool(m.cl)

	// Add stats
	m.addStats()

	// Alloc format context
	// We need to create an intermediate variable to avoid "cgo argument has Go pointer to Go pointer" errors
	var ctxFormat *avformat.Context
	if ret := avformat.AvformatAllocOutputContext2(&ctxFormat, o.Format, o.FormatName, o.URL); ret < 0 {
		err = fmt.Errorf("astilibav: avformat.AvformatAllocOutputContext2 on %+v failed: %w", o, NewAvError(ret))
		return
	}
	m.ctxFormat = ctxFormat

	// Make sure the format ctx is properly closed
	m.cl.Add(func() error {
		m.ctxFormat.AvformatFreeContext()
		return nil
	})

	// This is a file
	if m.ctxFormat.Flags()&avformat.AVFMT_NOFILE == 0 {
		// Open
		var ctxAvIO *avformat.AvIOContext
		if ret := avformat.AvIOOpen(&ctxAvIO, o.URL, avformat.AVIO_FLAG_WRITE); ret < 0 {
			err = fmt.Errorf("astilibav: avformat.AvIOOpen on %+v failed: %w", o, NewAvError(ret))
			return
		}

		// Set pb
		m.ctxFormat.SetPb(ctxAvIO)

		// Make sure the avio ctx is properly closed
		m.cl.Add(func() error {
			if ret := avformat.AvIOClosep(&ctxAvIO); ret < 0 {
				return fmt.Errorf("astilibav: avformat.AvIOClosep on %+v failed: %w", o, NewAvError(ret))
			}
			return nil
		})
	}
	return
}

// Close closes the muxer properly
func (m *Muxer) Close() error {
	return m.cl.Close()
}

func (m *Muxer) addStats() {
	// Get stats
	ss := m.c.Stats()
	ss = append(ss,
		astikit.StatOptions{
			Handler: m.statIncomingRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "pps",
			},
		},
		astikit.StatOptions{
			Handler: m.statOutgoingRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of bits going out per second",
				Label:       "Outgoing rate",
				Name:        StatNameOutgoingRate,
				Unit:        "bps",
			},
		},
		astikit.StatOptions{
			Handler: m.statProcessedRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "pps",
			},
		},
	)

	// Add stats
	m.BaseNode.AddStats(ss...)
}

// CtxFormat returns the format ctx
func (m *Muxer) CtxFormat() *avformat.Context {
	return m.ctxFormat
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	m.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to write header once
		var ret int
		m.o.Do(func() { ret = m.ctxFormat.AvformatWriteHeader(nil) })
		if ret < 0 {
			emitAvError(m, m.eh, ret, "m.ctxFormat.AvformatWriteHeader on %s failed", m.ctxFormat.Filename())
			return
		}

		// Write trailer once everything is done
		m.cl.Add(func() error {
			if ret := m.ctxFormat.AvWriteTrailer(); ret < 0 {
				return fmt.Errorf("m.ctxFormat.AvWriteTrailer on %s failed: %w", m.ctxFormat.Filename(), NewAvError(ret))
			}
			return nil
		})

		// Make sure to stop the chan properly
		defer m.c.Stop()

		// Start chan
		m.c.Start(m.Context())
	})
}

// MuxerPktHandler is an object that can handle a pkt for the muxer
type MuxerPktHandler struct {
	*Muxer
	o *avformat.Stream
}

// NewHandler creates
func (m *Muxer) NewPktHandler(o *avformat.Stream) *MuxerPktHandler {
	return &MuxerPktHandler{
		Muxer: m,
		o:     o,
	}
}

// HandlePkt implements the PktHandler interface
func (h *MuxerPktHandler) HandlePkt(p PktHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	h.cl.Do(func() {
		// Increment incoming rate
		h.statIncomingRate.Add(1)

		// Copy pkt
		pkt := h.p.get()
		if ret := pkt.AvPacketRef(p.Pkt); ret < 0 {
			emitAvError(h, h.eh, ret, "AvPacketRef failed")
			return
		}

		// Add to chan
		h.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			h.cl.Do(func() {
				// Handle pause
				defer h.HandlePause()

				// Make sure to close pkt
				defer h.p.put(pkt)

				// Increment processed rate
				h.statProcessedRate.Add(1)

				// Rescale timestamps
				pkt.AvPacketRescaleTs(p.Descriptor.TimeBase(), h.o.TimeBase())

				// Set stream index
				pkt.SetStreamIndex(h.o.Index())

				// Increment outgoing rate
				h.statOutgoingRate.Add(float64(pkt.Size() * 8))

				// Restamp
				if h.restamper != nil {
					h.restamper.Restamp(pkt)
				}

				// Write frame
				if ret := h.ctxFormat.AvInterleavedWriteFrame((*avformat.Packet)(unsafe.Pointer(pkt))); ret < 0 {
					emitAvError(h, h.eh, ret, "h.ctxFormat.AvInterleavedWriteFrame failed")
					return
				}
			})
		})
	})
}
