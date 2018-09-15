package main

import (
	"context"
	"fmt"
	"github.com/asticode/goav/avutil"
	"sync/atomic"

	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/go-astitools/sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
)

var countTransmuxer uint64

type transmuxer struct {
	*astiencoder.BaseNode
	h astilibav.MuxHandler
	q *astisync.CtxQueue
}

func newTransmuxer() *transmuxer {
	c := atomic.AddUint64(&countTransmuxer, 1)
	return &transmuxer{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: "Modifies packets for transmuxing purposes",
			Label:       fmt.Sprintf("Transmuxer #%d", c),
			Name:        fmt.Sprintf("transmuxer_%d", c),
		}),
		q: astisync.NewCtxQueue(),
	}
}

func (r *transmuxer) connect(h astilibav.MuxHandler) {
	// Set handler
	r.h = h

	// Connect nodes
	n := h.(astiencoder.Node)
	astiencoder.ConnectNodes(r, n)
}

// Start starts the transmuxer
func (r *transmuxer) Start(ctx context.Context, o astiencoder.StartOptions, t astiencoder.CreateTaskFunc) {
	r.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Start queue
		r.q.Start(r.Context(), func(p interface{}) {
			// Assert payload
			d := p.(transmuxerHandleData)

			// Modify packet
			d.pkt.SetStreamIndex(d.outStream.Index())
			d.pkt.SetDuration(avutil.AvRescaleQ(d.pkt.Duration(), d.inStream.TimeBase(), d.outStream.TimeBase()))
			d.pkt.SetDts(avutil.AvRescaleQRnd(d.pkt.Dts(), d.inStream.TimeBase(), d.outStream.TimeBase(), avutil.AV_ROUND_NEAR_INF|avutil.AV_ROUND_PASS_MINMAX))
			d.pkt.SetPts(avutil.AvRescaleQRnd(d.pkt.Pts(), d.inStream.TimeBase(), d.outStream.TimeBase(), avutil.AV_ROUND_NEAR_INF|avutil.AV_ROUND_PASS_MINMAX))
			d.pkt.SetPos(-1)

			// Handle packet
			r.h.HandlePkt(d.pkt)
		})
	})
}

type transmuxerHandleData struct {
	inStream  *avformat.Stream
	outStream *avformat.Stream
	pkt       *avcodec.Packet
}

// HandlePkt implements the astilibav.DemuxHandler interface
func (r *transmuxer) HandlePkt(pkt *avcodec.Packet, inStream, outStream *avformat.Stream) {
	r.q.SendAndWait(transmuxerHandleData{
		inStream:  inStream,
		outStream: outStream,
		pkt:       pkt,
	})
}
