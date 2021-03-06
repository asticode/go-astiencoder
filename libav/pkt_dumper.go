package astilibav

import "C"
import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"text/template"
	"unsafe"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avcodec"
)

var countPktDumper uint64

// PktDumper represents an object capable of dumping packets
type PktDumper struct {
	*astiencoder.BaseNode
	c                 *astikit.Chan
	cl                *astikit.Closer
	count             uint32
	eh                *astiencoder.EventHandler
	o                 PktDumperOptions
	p                 *pktPool
	statIncomingRate  *astikit.CounterRateStat
	statProcessedRate *astikit.CounterRateStat
	t                 *template.Template
}

// PktDumperOptions represents pkt dumper options
type PktDumperOptions struct {
	Data    map[string]interface{}
	Handler func(pkt *avcodec.Packet, args PktDumperHandlerArgs) error
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
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		cl:                c.NewChild(),
		eh:                eh,
		o:                 o,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pkt pool
	d.p = newPktPool(d.cl)

	// Add stats
	d.addStats()

	// Parse pattern
	if len(o.Pattern) > 0 {
		if d.t, err = template.New("").Parse(o.Pattern); err != nil {
			err = fmt.Errorf("astilibav: parsing pattern %s as template failed: %w", o.Pattern, err)
			return
		}
	}
	return
}

// Close closes the pkt dumper properly
func (d *PktDumper) Close() error {
	return d.cl.Close()
}

func (d *PktDumper) addStats() {
	// Get stats
	ss := d.c.Stats()
	ss = append(ss,
		astikit.StatOptions{
			Handler: d.statIncomingRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "pps",
			},
		},
		astikit.StatOptions{
			Handler: d.statProcessedRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "pps",
			},
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
	d.cl.Do(func() {
		// Increment incoming rate
		d.statIncomingRate.Add(1)

		// Copy pkt
		pkt := d.p.get()
		if ret := pkt.AvPacketRef(p.Pkt); ret < 0 {
			emitAvError(d, d.eh, ret, "AvPacketRef failed")
			return
		}

		// Add to chan
		d.c.Add(func() {
			// Handle pause
			defer d.HandlePause()

			// Make sure to close pkt
			defer d.p.put(pkt)

			// Increment processed rate
			d.statProcessedRate.Add(1)

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
}

// PktDumpFunc is a PktDumpFunc that dumps the packet to a file
var PktDumpFile = func(pkt *avcodec.Packet, args PktDumperHandlerArgs) (err error) {
	// Create file
	var f *os.File
	if f, err = os.Create(args.Pattern); err != nil {
		err = fmt.Errorf("astilibav: creating file %s failed: %w", args.Pattern, err)
		return
	}
	defer f.Close()

	// Write to file
	if _, err = f.Write(C.GoBytes(unsafe.Pointer(pkt.Data()), (C.int)(pkt.Size()))); err != nil {
		err = fmt.Errorf("astilibav: writing to file %s failed: %w", args.Pattern, err)
		return
	}
	return
}
