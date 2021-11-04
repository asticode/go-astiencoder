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
	c                 *astikit.Chan
	cl                *astikit.Closer
	ctxCodec          *avcodec.Context
	d                 *frameDispatcher
	eh                *astiencoder.EventHandler
	outputCtx         Context
	fp                *framePool
	pp                *pktPool
	statIncomingRate  *astikit.CounterRateStat
	statProcessedRate *astikit.CounterRateStat
}

// DecoderOptions represents decoder options
type DecoderOptions struct {
	CodecParams *avcodec.CodecParameters
	Name        string
	Node        astiencoder.NodeOptions
	OutputCtx   Context
}

// NewDecoder creates a new decoder
func NewDecoder(o DecoderOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (d *Decoder, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDecoder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("decoder_%d", count), fmt.Sprintf("Decoder #%d", count), "Decodes", "decoder")

	// Create decoder
	d = &Decoder{
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		cl:                c.NewChild(),
		eh:                eh,
		outputCtx:         o.OutputCtx,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pools
	d.fp = newFramePool(d.cl)
	d.pp = newPktPool(d.cl)

	// Create frame dispatcher
	d.d = newFrameDispatcher(d, eh, d.fp)

	// Add stats
	d.addStats()

	// Find decoder
	var cdc *avcodec.Codec
	if o.Name != "" {
		if cdc = avcodec.AvcodecFindDecoderByName(o.Name); cdc == nil {
			err = fmt.Errorf("astilibav: no decoder found for name %s", o.Name)
			return
		}
	} else {
		if cdc = avcodec.AvcodecFindDecoder(o.CodecParams.CodecId()); cdc == nil {
			err = fmt.Errorf("astilibav: no decoder found for codec id %+v", o.CodecParams.CodecId())
			return
		}
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
	d.cl.Add(func() error {
		if ret := d.ctxCodec.AvcodecClose(); ret < 0 {
			emitAvError(nil, eh, ret, "d.ctxCodec.AvcodecClose failed")
		}
		return nil
	})
	return
}

// Close closes the decoder properly
func (d *Decoder) Close() error {
	return d.cl.Close()
}

func (d *Decoder) addStats() {
	// Get stats
	ss := d.c.Stats()
	ss = append(ss, d.d.stats()...)
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
		// Make sure to stop the chan properly
		defer d.c.Stop()

		// Start chan
		d.c.Start(d.Context())
	})
}

// HandlePkt implements the PktHandler interface
func (d *Decoder) HandlePkt(p PktHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	d.cl.Do(func() {
		// Increment incoming rate
		d.statIncomingRate.Add(1)

		// Copy pkt
		pkt := d.pp.get()
		if ret := pkt.AvPacketRef(p.Pkt); ret < 0 {
			emitAvError(d, d.eh, ret, "AvPacketRef failed")
			return
		}

		// Add to chan
		d.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			d.cl.Do(func() {
				// Handle pause
				defer d.HandlePause()

				// Make sure to close pkt
				defer d.pp.put(pkt)

				// Increment processed rate
				d.statProcessedRate.Add(1)

				// Send pkt to decoder
				if ret := avcodec.AvcodecSendPacket(d.ctxCodec, pkt); ret < 0 {
					emitAvError(d, d.eh, ret, "avcodec.AvcodecSendPacket failed")
					return
				}

				// Loop
				for {
					// Receive frame
					if stop := d.receiveFrame(p.Descriptor); stop {
						return
					}
				}
			})
		})
	})
}

func (d *Decoder) receiveFrame(descriptor Descriptor) (stop bool) {
	// Get frame
	f := d.fp.get()
	defer d.fp.put(f)

	// Receive frame
	if ret := avcodec.AvcodecReceiveFrame(d.ctxCodec, f); ret < 0 {
		if ret != avutil.AVERROR_EOF && ret != avutil.AVERROR_EAGAIN {
			emitAvError(d, d.eh, ret, "avcodec.AvcodecReceiveFrame failed")
		}
		stop = true
		return
	}

	// Dispatch frame
	d.d.dispatch(f, descriptor)
	return
}
