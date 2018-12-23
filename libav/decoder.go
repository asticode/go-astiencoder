package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

var countDecoder uint64

// Decoder represents an object capable of decoding packets
type Decoder struct {
	*astiencoder.BaseNode
	ctxCodec         *avcodec.Context
	d                *frameDispatcher
	eh               *astiencoder.EventHandler
	q                *astisync.CtxQueue
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// DecoderOptions represents decoder options
type DecoderOptions struct {
	CodecParams *avcodec.CodecParameters
	Node        astiencoder.NodeOptions
}

// NewDecoder creates a new decoder
func NewDecoder(o DecoderOptions, eh *astiencoder.EventHandler, c *astiencoder.Closer) (d *Decoder, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDecoder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("decoder_%d", count), fmt.Sprintf("Decoder #%d", count), "Decodes")

	// Create decoder
	d = &Decoder{
		eh:               eh,
		q:                astisync.NewCtxQueue(),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	d.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(d), eh)
	d.d = newFrameDispatcher(d, eh, c)
	d.addStats()

	// Find decoder
	var cdc *avcodec.Codec
	if cdc = avcodec.AvcodecFindDecoder(o.CodecParams.CodecId()); cdc == nil {
		err = fmt.Errorf("astilibav: no decoder found for codec id %+v", o.CodecParams.CodecId())
		return
	}

	// Alloc context
	if d.ctxCodec = cdc.AvcodecAllocContext3(); d.ctxCodec == nil {
		err = fmt.Errorf("astilibav: no context allocated for codec %+v", cdc)
		return
	}

	// Copy codec parameters
	if ret := avcodec.AvcodecParametersToContext(d.ctxCodec, o.CodecParams); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: avcodec.AvcodecParametersToContext failed")
		return
	}

	// Open codec
	if ret := d.ctxCodec.AvcodecOpen2(cdc, nil); ret < 0 {
		err = errors.Wrap(NewAvError(ret), "astilibav: d.ctxCodec.AvcodecOpen2 failed")
		return
	}

	// Make sure the codec is closed
	c.Add(func() error {
		if ret := d.ctxCodec.AvcodecClose(); ret < 0 {
			emitAvError(nil, eh, ret, "d.ctxCodec.AvcodecClose failed")
		}
		return nil
	})
	return
}

func (d *Decoder) addStats() {
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

	// Add dispatcher stats
	d.d.addStats(d.Stater())

	// Add queue stats
	d.q.AddStats(d.Stater())
}

// Connect implements the FrameHandlerConnector interface
func (d *Decoder) Connect(h FrameHandler) {
	// Add handler
	d.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(d, h)
}

// Disconnect implements the FrameHandlerConnector interface
func (d *Decoder) Disconnect(h FrameHandler) {
	// Delete handler
	d.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(d, h)
}

// Start starts the decoder
func (d *Decoder) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	d.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go d.q.HandleCtx(d.Context())

		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer d.d.wait()

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

			// Send pkt to decoder
			d.statWorkRatio.Add(true)
			if ret := avcodec.AvcodecSendPacket(d.ctxCodec, p.Pkt); ret < 0 {
				d.statWorkRatio.Done(true)
				emitAvError(d, d.eh, ret, "avcodec.AvcodecSendPacket failed")
				return
			}
			d.statWorkRatio.Done(true)

			// Loop
			for {
				// Receive frame
				if stop := d.receiveFrame(p.Descriptor); stop {
					return
				}
			}
		})
	})
}

func (d *Decoder) receiveFrame(descriptor Descriptor) (stop bool) {
	// Get frame
	f := d.d.p.get()
	defer d.d.p.put(f)

	// Receive frame
	d.statWorkRatio.Add(true)
	if ret := avcodec.AvcodecReceiveFrame(d.ctxCodec, f); ret < 0 {
		d.statWorkRatio.Done(true)
		if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
			emitAvError(d, d.eh, ret, "avcodec.AvcodecReceiveFrame failed")
		}
		stop = true
		return
	}
	d.statWorkRatio.Done(true)

	// Dispatch frame
	d.d.dispatch(f, descriptor)
	return
}

// HandlePkt implements the PktHandler interface
func (d *Decoder) HandlePkt(p *PktHandlerPayload) {
	d.q.Send(p)
}
