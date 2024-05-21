package astilibav

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countDecoder uint64

// Decoder represents an object capable of decoding packets
type Decoder struct {
	*astiencoder.BaseNode
	c                    *astikit.Chan
	codecCtx             *astiav.CodecContext
	d                    *frameDispatcher
	eh                   *astiencoder.EventHandler
	enforceMonotonicDTS  bool
	fp                   *framePool
	outputCtx            Context
	previousDts          *int64
	statBytesReceived    uint64
	statPacketsProcessed uint64
	statPacketsReceived  uint64
	pp                   *pktPool
}

// DecoderOptions represents decoder options
type DecoderOptions struct {
	CodecParameters     *astiav.CodecParameters
	EnforceMonotonicDTS bool
	Name                string
	Node                astiencoder.NodeOptions
	OutputCtx           Context
	ThreadCount         int
	ThreadType          astiav.ThreadType
}

// NewDecoder creates a new decoder
func NewDecoder(o DecoderOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (d *Decoder, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countDecoder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("decoder_%d", count), fmt.Sprintf("Decoder #%d", count), "Decodes", "decoder")

	// Create decoder
	d = &Decoder{
		c:                   astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:                  eh,
		enforceMonotonicDTS: o.EnforceMonotonicDTS,
		outputCtx:           o.OutputCtx,
	}

	// Create base node
	d.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, d, astiencoder.EventTypeToNodeEventName)

	// Create pools
	d.fp = newFramePool(d.NewChildCloser())
	d.pp = newPktPool(d)

	// Create frame dispatcher
	d.d = newFrameDispatcher(d, eh)

	// Add stat options
	d.addStatOptions()

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

	// Set thread parameters
	if o.ThreadCount > 0 {
		d.codecCtx.SetThreadCount(o.ThreadCount)
	}
	if o.ThreadType != astiav.ThreadTypeUndefined {
		d.codecCtx.SetThreadType(o.ThreadType)
	}

	// Open codec
	if err = d.codecCtx.Open(codec, nil); err != nil {
		err = fmt.Errorf("astilibav: opening codec failed: %w", err)
		return
	}
	return
}

type DecoderStats struct {
	BytesReceived    uint64
	FramesAllocated  uint64
	FramesDispatched uint64
	PacketsAllocated uint64
	PacketsProcessed uint64
	PacketsReceived  uint64
	WorkDuration     time.Duration
}

func (d *Decoder) Stats() DecoderStats {
	return DecoderStats{
		BytesReceived:    atomic.LoadUint64(&d.statBytesReceived),
		FramesAllocated:  d.fp.stats().framesAllocated,
		FramesDispatched: d.d.stats().framesDispatched,
		PacketsAllocated: d.pp.stats().packetsAllocated,
		PacketsProcessed: atomic.LoadUint64(&d.statPacketsProcessed),
		PacketsReceived:  atomic.LoadUint64(&d.statPacketsReceived),
		WorkDuration:     d.c.Stats().WorkDuration,
	}
}

func (d *Decoder) addStatOptions() {
	// Get stat options
	ss := d.c.StatOptions()
	ss = append(ss, d.d.statOptions()...)
	ss = append(ss, d.fp.statOptions()...)
	ss = append(ss, d.pp.statOptions()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&d.statPacketsReceived),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "pps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&d.statPacketsProcessed),
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
		// Increment packets received
		atomic.AddUint64(&d.statPacketsReceived, 1)

		// Increment received bytes
		if p.Pkt != nil {
			atomic.AddUint64(&d.statBytesReceived, uint64(p.Pkt.Size()))
		}

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

				// Increment packets processed
				atomic.AddUint64(&d.statPacketsProcessed, 1)

				// Enforce monotonic dts
				if d.enforceMonotonicDTS && d.previousDts != nil && *d.previousDts >= pkt.Dts() {
					emitError(d, d.eh, errors.New("astilibav: previous dts >= current dts"), "enforcing monotonic dts")
					return
				}
				d.previousDts = astikit.Int64Ptr(pkt.Dts())

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
