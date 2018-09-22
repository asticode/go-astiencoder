package astilibav

import (
	"bytes"
	"context"
	"fmt"
	"github.com/asticode/go-astilog"
	"sync/atomic"
	"text/template"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/pkg/errors"
)

var countPktDumper uint64

// PktDumper represents an object capable of dumping packets
type PktDumper struct {
	*astiencoder.BaseNode
	count   uint32
	data    map[string]interface{}
	e       astiencoder.EmitEventFunc
	fn      PktDumpFunc
	pattern string
	q       *astisync.CtxQueue
	t       *template.Template
}

// PktDumpFunc represents a pkt dump func
type PktDumpFunc func(pkt *avcodec.Packet, pattern string)

// NewPktDumper creates a new pk dumper
func NewPktDumper(pattern string, fn PktDumpFunc, data map[string]interface{}, e astiencoder.EmitEventFunc) (d *PktDumper, err error) {
	// Create pkt dumper
	count := atomic.AddUint64(&countPktDumper, uint64(1))
	d = &PktDumper{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: "Dump packets",
			Label:       fmt.Sprintf("Pkt dumper #%d", count),
			Name:        fmt.Sprintf("pkt_dumper_%d", count),
		}),
		data:    data,
		e:       e,
		fn:      fn,
		pattern: pattern,
		q:       astisync.NewCtxQueue(),
	}

	// Parse pattern
	if d.t, err = template.New("").Parse(pattern); err != nil {
		err = errors.Wrapf(err, "astilibav: parsing pattern %s as template failed", pattern)
		return
	}
	return
}

// Start starts the pkt dumper
func (d *PktDumper) Start(ctx context.Context, o astiencoder.WorkflowStartOptions, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Handle context
		go d.q.HandleCtx(d.Context())

		// Start queue
		d.q.Start(func(p interface{}) {
			// Assert payload
			pkt := p.(*avcodec.Packet)

			// Increment count
			c := atomic.AddUint32(&d.count, 1)

			// Create data
			d.data["count"] = c
			d.data["pts"] = pkt.Pts()
			d.data["stream_idx"] = pkt.StreamIndex()

			// Execute template
			buf := &bytes.Buffer{}
			if err := d.t.Execute(buf, d.data); err != nil {
				d.e(astiencoder.EventError(errors.Wrapf(err, "astilibav: executing template %s with data %+v failed", d.pattern, d.data)))
				return
			}

			// Dump
			d.fn(pkt, buf.String())
		})
	})
}

// HandlePkt implements the PktHandler interface
func (d *PktDumper) HandlePkt(pkt *avcodec.Packet) {
	d.q.Send(pkt, true)
}

// PktDumpFunc is a PktDumpFunc that dumps the packet to a file
var PktDumpFile = func(pkt *avcodec.Packet, pattern string) {
	// TODO Write to file
	astilog.Warnf("writing pkt to %s", pattern)
	// http://www.cplusplus.com/reference/cstdio/fwrite/
}
