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
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/pkg/errors"
)

var countPktDumper uint64

// PktDumper represents an object capable of dumping packets
type PktDumper struct {
	*astiencoder.BaseNode
	count            uint32
	e                astiencoder.EventEmitter
	o                PktDumperOptions
	q                *astisync.CtxQueue
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
	t                *template.Template
}

// PktDumperOptions represents pkt dumper options
type PktDumperOptions struct {
	Data    map[string]interface{}
	Handler func(pkt *avcodec.Packet, args PktDumperHandlerArgs) error
	Node astiencoder.NodeOptions
	Pattern string
}

// PktDumperHandlerArgs represents pkt dumper handler args
type PktDumperHandlerArgs struct {
	Pattern string
}

// NewPktDumper creates a new pk dumper
func NewPktDumper(o PktDumperOptions, e astiencoder.EventEmitter) (d *PktDumper, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countPktDumper, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("pkt_dumper_%d", count), fmt.Sprintf("Pkt Dumper #%d", count), "Dumps packets")

	// Create pkt dumper
	d = &PktDumper{
		e:                e,
		o:                o,
		q:                astisync.NewCtxQueue(),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	d.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(d), e)
	d.addStats()

	// Parse pattern
	if len(o.Pattern) > 0 {
		if d.t, err = template.New("").Parse(o.Pattern); err != nil {
			err = errors.Wrapf(err, "astilibav: parsing pattern %s as template failed", o.Pattern)
			return
		}
	}
	return
}

func (d *PktDumper) addStats() {
	// Add incoming rate
	d.Stater().AddStat(astistat.StatMetadata{
		Description: "Number of packets coming in per second",
		Label:       "Incoming rate",
		Unit:        "pps",
	}, d.statIncomingRate)

	// Add work ratio
	d.Stater().AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, d.statWorkRatio)

	// Add queue stats
	d.q.AddStats(d.Stater())
}

// Start starts the pkt dumper
func (d *PktDumper) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go d.q.HandleCtx(d.Context())

		// Make sure to stop the queue properly
		defer d.q.Stop()

		// Start queue
		d.q.Start(func(dp interface{}) {
			// Handle pause
			defer d.HandlePause()

			// Assert payload
			p := dp.(*PktHandlerPayload)

			// Increment incoming rate
			d.statIncomingRate.Add(1)

			// Get pattern
			var args PktDumperHandlerArgs
			if d.t != nil {
				// Increment count
				c := atomic.AddUint32(&d.count, 1)

				// Create data
				d.o.Data["count"] = c
				d.o.Data["pts"] = p.Pkt.Pts()
				d.o.Data["stream_idx"] = p.Pkt.StreamIndex()

				// Execute template
				d.statWorkRatio.Add(true)
				buf := &bytes.Buffer{}
				if err := d.t.Execute(buf, d.o.Data); err != nil {
					d.statWorkRatio.Done(true)
					d.e.Emit(astiencoder.EventError(d, errors.Wrapf(err, "astilibav: executing template %s with data %+v failed", d.o.Pattern, d.o.Data)))
					return
				}
				d.statWorkRatio.Done(true)

				// Add to args
				args.Pattern = buf.String()
			}

			// Dump
			d.statWorkRatio.Add(true)
			if err := d.o.Handler(p.Pkt, args); err != nil {
				d.statWorkRatio.Done(true)
				d.e.Emit(astiencoder.EventError(d, errors.Wrapf(err, "astilibav: pkt dump func with args %+v failed", args)))
				return
			}
			d.statWorkRatio.Done(true)
		})
	})
}

// HandlePkt implements the PktHandler interface
func (d *PktDumper) HandlePkt(p *PktHandlerPayload) {
	d.q.Send(p)
}

// PktDumpFunc is a PktDumpFunc that dumps the packet to a file
var PktDumpFile = func(pkt *avcodec.Packet, args PktDumperHandlerArgs) (err error) {
	// Create file
	var f *os.File
	if f, err = os.Create(args.Pattern); err != nil {
		err = errors.Wrapf(err, "astilibav: creating file %s failed", args.Pattern)
		return
	}
	defer f.Close()

	// Write to file
	if _, err = f.Write(C.GoBytes(unsafe.Pointer(pkt.Data()), (C.int)(pkt.Size()))); err != nil {
		err = errors.Wrapf(err, "astilibav: writing to file %s failed", args.Pattern)
		return
	}
	return
}
