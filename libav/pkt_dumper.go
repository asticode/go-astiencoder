package astilibav

import "C"
import (
	"bytes"
	"context"
	"fmt"
	"os"
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
	o                    PktDumperOptions
	p                    *pktPool
	statPacketsProcessed uint64
	statPacketsReceived  uint64
	t                    *template.Template
}

// PktDumperOptions represents pkt dumper options
type PktDumperOptions struct {
	Data    map[string]interface{}
	Handler func(pkt *astiav.Packet, args PktDumperHandlerArgs) error
	Node    astiencoder.NodeOptions
	Pattern string
}

// PktDumperHandlerArgs represents pkt dumper handler args
type PktDumperHandlerArgs struct {
	Pattern string
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
		o:  o,
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pkt pool
	d.p = newPktPool(d)

	// Add stat options
	d.addStatOptions()

	// Parse pattern
	if len(o.Pattern) > 0 {
		if d.t, err = template.New("").Parse(o.Pattern); err != nil {
			err = fmt.Errorf("astilibav: parsing pattern %s as template failed: %w", o.Pattern, err)
			return
		}
	}
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

				// Get pattern
				var args PktDumperHandlerArgs
				if d.t != nil {
					// Increment count
					c := atomic.AddUint32(&d.count, 1)

					// Create data
					data := make(map[string]interface{})
					if d.o.Data != nil {
						data = d.o.Data
					}
					data["count"] = c
					data["pts"] = pkt.Pts()
					data["stream_idx"] = pkt.StreamIndex()

					// Execute template
					buf := &bytes.Buffer{}
					if err := d.t.Execute(buf, data); err != nil {
						d.eh.Emit(astiencoder.EventError(d, fmt.Errorf("astilibav: executing template %s with data %+v failed: %w", d.o.Pattern, d.o.Data, err)))
						return
					}

					// Add to args
					args.Pattern = buf.String()
				}

				// Dump
				if err := d.o.Handler(pkt, args); err != nil {
					d.eh.Emit(astiencoder.EventError(d, fmt.Errorf("astilibav: pkt dump func with args %+v failed: %w", args, err)))
					return
				}
			})
		})
	})
}

// PktDumpFunc is a PktDumpFunc that dumps the packet to a file
var PktDumpFile = func(pkt *astiav.Packet, args PktDumperHandlerArgs) (err error) {
	// Create file
	var f *os.File
	if f, err = os.Create(args.Pattern); err != nil {
		err = fmt.Errorf("astilibav: creating file %s failed: %w", args.Pattern, err)
		return
	}
	defer f.Close()

	// Write to file
	if _, err = f.Write(pkt.Data()); err != nil {
		err = fmt.Errorf("astilibav: writing to file %s failed: %w", args.Pattern, err)
		return
	}
	return
}
