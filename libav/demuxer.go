package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
)

var countDemuxer uint64

// Demuxer represents an object capable of demuxing packets out of an input
type Demuxer struct {
	*astiencoder.BaseNode
	ctxFormat           *avformat.Context
	d                   *pktDispatcher
	e                   astiencoder.EmitEventFunc
	packetsBufferLength int
}

// NewDemuxer creates a new demuxer
func NewDemuxer(ctxFormat *avformat.Context, e astiencoder.EmitEventFunc, c *astiencoder.Closer, packetsBufferLength int) *Demuxer {
	count := atomic.AddUint64(&countDemuxer, uint64(1))
	return &Demuxer{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: fmt.Sprintf("Demuxes %s", ctxFormat.Filename()),
			Label:       fmt.Sprintf("Demuxer #%d", count),
			Name:        fmt.Sprintf("demuxer_%d", count),
		}),
		ctxFormat:           ctxFormat,
		d:                   newPktDispatcher(c),
		e:                   e,
		packetsBufferLength: packetsBufferLength,
	}
}

// Connect connects the demuxer to a PktHandler for a specific stream index
func (d *Demuxer) Connect(i *avformat.Stream, h PktHandler) {
	// Add handler
	d.d.addHandler(h, func(pkt *avcodec.Packet) bool {
		return pkt.StreamIndex() == i.Index()
	})

	// Connect nodes
	astiencoder.ConnectNodes(d, h.(astiencoder.Node))
}

// Start starts the demuxer
func (d *Demuxer) Start(ctx context.Context, o astiencoder.WorkflowStartOptions, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Create regulator
		r := astisync.NewRegulator(d.Context(), d.packetsBufferLength)
		defer r.Wait()

		// Loop
		for {
			// Read frame
			if ret := d.ctxFormat.AvReadFrame(d.d.pkt); ret < 0 {
				if ret != avutil.AVERROR_EOF {
					emitAvError(d.e, ret, "ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
				}
				return
			}

			// Dispatch pkt
			d.d.dispatch(r)

			// Check context
			if d.Context().Err() != nil {
				return
			}
		}
	})
}
