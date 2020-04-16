package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
)

var countDecoder uint64

// Decoder represents an object capable of decoding packets
type Decoder struct {
	*astiencoder.BaseNode
	c                *astikit.Chan
	ctxCodec         *avcodec.Context
	d                *frameDispatcher
	eh               *astiencoder.EventHandler
	outputCtx        Context
	statIncomingRate *astikit.CounterAvgStat
	statWorkRatio    *astikit.DurationPercentageStat
}

// DecoderOptions represents decoder options
type DecoderOptions struct {
	CodecParams *avcodec.CodecParameters
	Node        astiencoder.NodeOptions
	OutputCtx   Context
}

// NewDecoder creates a new decoder
func NewDecoder(o DecoderOptions, eh *astiencoder.EventHandler, c *astikit.Closer) (d *Decoder, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDecoder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("decoder_%d", count), fmt.Sprintf("Decoder #%d", count), "Decodes")

	// Create decoder
	d = &Decoder{
		c: astikit.NewChan(astikit.ChanOptions{
			AddStrategy: astikit.ChanAddStrategyBlockWhenStarted,
			ProcessAll:  true,
		}),
		eh:               eh,
		outputCtx:        o.OutputCtx,
		statIncomingRate: astikit.NewCounterAvgStat(),
		statWorkRatio:    astikit.NewDurationPercentageStat(),
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
		err = fmt.Errorf("astilibav: avcodec.AvcodecParametersToContext failed: %w", NewAvError(ret))
		return
	}

	// Open codec
	if ret := d.ctxCodec.AvcodecOpen2(cdc, nil); ret < 0 {
		err = fmt.Errorf("astilibav: d.ctxCodec.AvcodecOpen2 failed: %w", NewAvError(ret))
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
	d.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of packets coming in per second",
		Label:       "Incoming rate",
		Unit:        "pps",
	}, d.statIncomingRate)

	// Add work ratio
	d.Stater().AddStat(astikit.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, d.statWorkRatio)

	// Add dispatcher stats
	d.d.addStats(d.Stater())

	// Add chan stats
	d.c.AddStats(d.Stater())
}

// OutputCtx returns the output ctx
func (d *Decoder) OutputCtx() Context {
	return d.outputCtx
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
	d.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer d.d.wait()

		// Make sure to stop the chan properly
		defer d.c.Stop()

		// Start chan
		d.c.Start(d.Context())
	})
}

// HandlePkt implements the PktHandler interface
func (d *Decoder) HandlePkt(p *PktHandlerPayload) {
	d.c.Add(func() {
		// Handle pause
		defer d.HandlePause()

		// Increment incoming rate
		d.statIncomingRate.Add(1)

		// Send pkt to decoder
		d.statWorkRatio.Begin()
		if ret := avcodec.AvcodecSendPacket(d.ctxCodec, p.Pkt); ret < 0 {
			d.statWorkRatio.End()
			emitAvError(d, d.eh, ret, "avcodec.AvcodecSendPacket failed")
			return
		}
		d.statWorkRatio.End()

		// Loop
		for {
			// Receive frame
			if stop := d.receiveFrame(p.Descriptor); stop {
				return
			}
		}
	})
}

func (d *Decoder) receiveFrame(descriptor Descriptor) (stop bool) {
	// Get frame
	f := d.d.p.get()
	defer d.d.p.put(f)

	// Receive frame
	d.statWorkRatio.Begin()
	if ret := avcodec.AvcodecReceiveFrame(d.ctxCodec, f); ret < 0 {
		d.statWorkRatio.End()
		if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
			emitAvError(d, d.eh, ret, "avcodec.AvcodecReceiveFrame failed")
		}
		stop = true
		return
	}
	d.statWorkRatio.End()

	// Dispatch frame
	d.d.dispatch(f, descriptor)
	return
}
