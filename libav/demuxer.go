package astilibav

import (
	"context"
	"fmt"
	"sync"
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
	c                   *astiencoder.Closer
	ctxFormat           *avformat.Context
	e                   astiencoder.EmitEventFunc
	hs                  map[int][]demuxerHandlerData // Indexed by stream index
	m                   *sync.Mutex
	packetsBufferLength int
}

type demuxerHandlerData struct {
	h   PktHandler
	pkt *avcodec.Packet
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
		c:                   c,
		ctxFormat:           ctxFormat,
		e:                   e,
		hs:                  make(map[int][]demuxerHandlerData),
		packetsBufferLength: packetsBufferLength,
		m:                   &sync.Mutex{},
	}
}

// Connect connects the demuxer to a PktHandler for a specific stream index
func (d *Demuxer) Connect(i *avformat.Stream, h PktHandler) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Create pkt
	pkt := avcodec.AvPacketAlloc()
	d.c.Add(func() error {
		avcodec.AvPacketFree(pkt)
		return nil
	})

	// Append handler
	d.hs[i.Index()] = append(d.hs[i.Index()], demuxerHandlerData{
		h:   h,
		pkt: pkt,
	})

	// Connect nodes
	n := h.(astiencoder.Node)
	astiencoder.ConnectNodes(d, n)
}

// Start starts the demuxer
func (d *Demuxer) Start(ctx context.Context, o astiencoder.WorkflowStartOptions, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Create regulator
		r := astisync.NewRegulator(d.Context(), d.packetsBufferLength)
		defer r.Wait()

		// Loop
		var pkt = &avcodec.Packet{}
		for {
			// Read frame
			if ret := d.ctxFormat.AvReadFrame(pkt); ret < 0 {
				if ret != avutil.AVERROR_EOF {
					emitAvError(d.e, ret, "ctxFormat.AvReadFrame on %s failed", d.ctxFormat.Filename())
				}
				return
			}

			// Handle packet
			d.handlePkt(pkt, r)

			// Check context
			if d.Context().Err() != nil {
				return
			}
		}
	})
}

func (d *Demuxer) handlePkt(pkt *avcodec.Packet, r *astisync.Regulator) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Make sure the pkt is unref
	defer pkt.AvPacketUnref()

	// Retrieve handlers
	hs, ok := d.hs[pkt.StreamIndex()]
	if !ok {
		return
	}

	// Create new process
	p := r.NewProcess()

	// Add subprocesses
	p.AddSubprocesses(len(hs), nil)

	// Loop through handlers
	for _, h := range hs {
		// Copy pkt
		h.pkt.AvPacketRef(pkt)

		// Handle pkt
		go func(h demuxerHandlerData) {
			defer p.SubprocessIsDone()
			defer h.pkt.AvPacketUnref()
			h.h.HandlePkt(h.pkt)
		}(h)
	}

	// Wait for one of the subprocess to be done
	p.Wait()
}
