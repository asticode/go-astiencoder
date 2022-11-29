package astilibav

import "C"
import (
	"context"
	"fmt"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countPktDumper uint64

// PktDumper represents an object capable of dumping packets
type PktDumper struct {
	*astiencoder.BaseNode
	c                    *astikit.Chan
	count                uint32
	eh                   *astiencoder.EventHandler
	h                    PktDumperHandler
	p                    *pktPool
	statPacketsProcessed uint64
	statPacketsReceived  uint64
	t                    *template.Template
}

// PktDumperHandler represents a pkt dumper handler
type PktDumperHandler func(pkt *astiav.Packet)

// PktDumperOptions represents pkt dumper options
type PktDumperOptions struct {
	Handler PktDumperHandler
	Node    astiencoder.NodeOptions
}

// NewPktDumper creates a new pk dumper
func NewPktDumper(o PktDumperOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (d *PktDumper, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countPktDumper, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("pkt_dumper_%d", count), fmt.Sprintf("Pkt Dumper #%d", count), "Dumps packets", "pkt dumper")

	// Create pkt dumper
	d = &PktDumper{
		c:  astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh: eh,
		h:  o.Handler,
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pkt pool
	d.p = newPktPool(d)

	// Add stat options
	d.addStatOptions()
	return
}

type PktDumperStats struct {
	PacketsAllocated uint64
	PacketsProcessed uint64
	PacketsReceived  uint64
	WorkDuration     time.Duration
}

func (d *PktDumper) Stats() PktDumperStats {
	return PktDumperStats{
		PacketsAllocated: d.p.stats().packetsAllocated,
		PacketsProcessed: atomic.LoadUint64(&d.statPacketsProcessed),
		PacketsReceived:  atomic.LoadUint64(&d.statPacketsReceived),
		WorkDuration:     d.c.Stats().WorkDuration,
	}
}

func (d *PktDumper) addStatOptions() {
	// Get stats
	ss := d.c.StatOptions()
	ss = append(ss, d.p.statOptions()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&d.statPacketsReceived),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&d.statPacketsProcessed),
		},
	)

	// Add stats
	d.BaseNode.AddStats(ss...)
}

// Start starts the pkt dumper
func (d *PktDumper) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to stop the chan properly
		defer d.c.Stop()

		// Start chan
		d.c.Start(d.Context())
	})
}

// HandlePkt implements the PktHandler interface
func (d *PktDumper) HandlePkt(p PktHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	d.DoWhenUnclosed(func() {
		// Increment received packets
		atomic.AddUint64(&d.statPacketsReceived, 1)

		// Copy pkt
		pkt := d.p.get()
		if err := pkt.Ref(p.Pkt); err != nil {
			emitError(d, d.eh, err, "refing packet")
			return
		}

		// Add to chan
		d.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			d.DoWhenUnclosed(func() {
				// Handle pause
				defer d.HandlePause()

				// Make sure to close pkt
				defer d.p.put(pkt)

				// Increment processed packets
				atomic.AddUint64(&d.statPacketsProcessed, 1)

				// Handle
				if d.h != nil {
					d.h(pkt)
				}
			})
		})
	})
}
