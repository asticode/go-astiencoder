package astilibav

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countDecoder uint64

// Decoder represents an object capable of decoding packets
type Decoder struct {
	*astiencoder.BaseNode
	c                 *astikit.Chan
	codecCtx          *astiav.CodecContext
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
	CodecParameters *astiav.CodecParameters
	Name            string
	Node            astiencoder.NodeOptions
	OutputCtx       Context
}

// NewDecoder creates a new decoder
func NewDecoder(o DecoderOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (d *Decoder, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDecoder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("decoder_%d", count), fmt.Sprintf("Decoder #%d", count), "Decodes", "decoder")

	// Create decoder
	d = &Decoder{
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:                eh,
		outputCtx:         o.OutputCtx,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pools
	d.fp = newFramePool(d)
	d.pp = newPktPool(d)

	// Create frame dispatcher
	d.d = newFrameDispatcher(d, eh)

	// Add stats
	d.addStats()

	// Find decoder
	var codec *astiav.Codec
	if o.Name != "" {
		if codec = astiav.FindDecoderByName(o.Name); codec == nil {
			err = fmt.Errorf("astilibav: no decoder found for name %s", o.Name)
			return
		}
	} else {
		if codec = astiav.FindDecoder(o.CodecParameters.CodecID()); codec == nil {
			err = fmt.Errorf("astilibav: no decoder found for codec id %s", o.CodecParameters.CodecID())
			return
		}
	}

	// Alloc codec context
	if d.codecCtx = astiav.AllocCodecContext(codec); d.codecCtx == nil {
		err = errors.New("astilibav: no codec context allocated")
		return
	}

	// Make sure the codec context is freed
	d.AddClose(d.codecCtx.Free)

	// Convert codec parameters to codec context
	if err = o.CodecParameters.ToCodecContext(d.codecCtx); err != nil {
		err = fmt.Errorf("astilibav: converting codec parameters to codec context failed: %w", err)
		return
	}

	// Open codec
	if err = d.codecCtx.Open(codec, nil); err != nil {
		err = fmt.Errorf("astilibav: opening codec failed: %w", err)
		return
	}
	return
}

func (d *Decoder) addStats() {
	// Get stats
	ss := d.c.Stats()
	ss = append(ss, d.d.stats()...)
	ss = append(ss, d.fp.stats()...)
	ss = append(ss, d.pp.stats()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "pps",
			},
			Valuer: d.statIncomingRate,
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "pps",
			},
			Valuer: d.statProcessedRate,
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
	d.DoWhenUnclosed(func() {
		// Increment incoming rate
		d.statIncomingRate.Add(1)

		// Copy pkt
		pkt := d.pp.get()
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
				defer d.pp.put(pkt)

				// Increment processed rate
				d.statProcessedRate.Add(1)

				// Send pkt to decoder
				if err := d.codecCtx.SendPacket(pkt); err != nil {
					emitError(d, d.eh, err, "sending packet")
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
	if err := d.codecCtx.ReceiveFrame(f); err != nil {
		if !errors.Is(err, astiav.ErrEof) && !errors.Is(err, astiav.ErrEagain) {
			emitError(d, d.eh, err, "receiving frame")
		}
		stop = true
		return
	}

	// Dispatch frame
	d.d.dispatch(f, descriptor)
	return
}
