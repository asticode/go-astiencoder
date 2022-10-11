package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countMuxer uint64

// Muxer represents an object capable of muxing packets into an output
type Muxer struct {
	*astiencoder.BaseNode
	c                 *astikit.Chan
	formatContext     *astiav.FormatContext
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
	Format     *astiav.OutputFormat
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
		eh:                eh,
		o:                 &sync.Once{},
		restamper:         o.Restamper,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statOutgoingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	m.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, m, astiencoder.EventTypeToNodeEventName)

	// Create pkt pool
	m.p = newPktPool(m)

	// Add stats
	m.addStats()

	// Alloc format context
	if m.formatContext, err = astiav.AllocOutputFormatContext(o.Format, o.FormatName, o.URL); err != nil {
		err = fmt.Errorf("astilibav: allocating output format context failed: %w", err)
		return
	}

	// Make sure the format context is properly closed
	m.AddClose(m.formatContext.Free)

	// We need to use an io context if this is a file
	if !m.formatContext.OutputFormat().Flags().Has(astiav.IOFormatFlagNofile) {
		// Create io context
		ioContext := astiav.NewIOContext()

		// Open
		if err = ioContext.Open(o.URL, astiav.NewIOContextFlags(astiav.IOContextFlagWrite)); err != nil {
			err = fmt.Errorf("astilibav: opening io context failed: %w", err)
			return
		}

		// Make sure the io context is properly closed
		m.AddCloseWithError(func() error {
			if err := ioContext.Closep(); err != nil {
				return fmt.Errorf("astilibav: closing io context failed: %w", err)
			}
			return nil
		})

		// Set pb
		m.formatContext.SetPb(ioContext)
	}
	return
}

func (m *Muxer) addStats() {
	// Get stats
	ss := m.c.Stats()
	ss = append(ss, m.p.stats()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "pps",
			},
			Valuer: m.statIncomingRate,
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of bits going out per second",
				Label:       "Outgoing rate",
				Name:        StatNameOutgoingRate,
				Unit:        "bps",
			},
			Valuer: m.statOutgoingRate,
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "pps",
			},
			Valuer: m.statProcessedRate,
		},
	)

	// Add stats
	m.BaseNode.AddStats(ss...)
}

func (m *Muxer) FormatContext() *astiav.FormatContext {
	return m.formatContext
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	m.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to write header once
		var err error
		m.o.Do(func() { err = m.formatContext.WriteHeader(nil) })
		if err != nil {
			emitError(m, m.eh, err, "writing header")
			return
		}

		// Write trailer once everything is done
		m.AddCloseWithError(func() error {
			if err := m.formatContext.WriteTrailer(); err != nil {
				return fmt.Errorf("writing trailer failed: %w", err)
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
	o *astiav.Stream
}

// NewHandler creates
func (m *Muxer) NewPktHandler(o *astiav.Stream) *MuxerPktHandler {
	return &MuxerPktHandler{
		Muxer: m,
		o:     o,
	}
}

// HandlePkt implements the PktHandler interface
func (h *MuxerPktHandler) HandlePkt(p PktHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	h.DoWhenUnclosed(func() {
		// Increment incoming rate
		h.statIncomingRate.Add(1)

		// Copy pkt
		pkt := h.p.get()
		if err := pkt.Ref(p.Pkt); err != nil {
			emitError(h, h.eh, err, "refing packet")
			return
		}

		// Add to chan
		h.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			h.DoWhenUnclosed(func() {
				// Handle pause
				defer h.HandlePause()

				// Make sure to close pkt
				defer h.p.put(pkt)

				// Increment processed rate
				h.statProcessedRate.Add(1)

				// Rescale timestamps
				pkt.RescaleTs(p.Descriptor.TimeBase(), h.o.TimeBase())

				// Set stream index
				pkt.SetStreamIndex(h.o.Index())

				// Increment outgoing rate
				h.statOutgoingRate.Add(float64(pkt.Size() * 8))

				// Restamp
				if h.restamper != nil {
					h.restamper.Restamp(pkt)
				}

				// Write frame
				if err := h.formatContext.WriteInterleavedFrame(pkt); err != nil {
					emitError(h, h.eh, err, "writing interleaved frame")
					return
				}
			})
		})
	})
}
