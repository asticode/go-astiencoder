package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countForwarder uint64

// Forwarder represents an object capable of forwarding frames
type Forwarder struct {
	*astiencoder.BaseNode
	c                 *astikit.Chan
	d                 *frameDispatcher
	eh                *astiencoder.EventHandler
	outputCtx         Context
	p                 *framePool
	restamper         FrameRestamper
	statIncomingRate  *astikit.CounterRateStat
	statProcessedRate *astikit.CounterRateStat
}

// ForwarderOptions represents forwarder options
type ForwarderOptions struct {
	Node      astiencoder.NodeOptions
	OutputCtx Context
	Restamper FrameRestamper
}

// NewForwarder creates a new forwarder
func NewForwarder(o ForwarderOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (f *Forwarder) {
	// Extend node metadata
	count := atomic.AddUint64(&countForwarder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("forwarder_%d", count), fmt.Sprintf("Forwarder #%d", count), "Forwards", "forwarder")

	// Create forwarder
	f = &Forwarder{
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:                eh,
		outputCtx:         o.OutputCtx,
		restamper:         o.Restamper,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	f.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, f, astiencoder.EventTypeToNodeEventName)

	// Create frame pool
	f.p = newFramePool(f)

	// Create frame dispatcher
	f.d = newFrameDispatcher(f, eh)

	// Add stats
	f.addStats()
	return
}

func (f *Forwarder) addStats() {
	// Get stats
	ss := f.c.Stats()
	ss = append(ss, f.d.stats()...)
	ss = append(ss, f.p.stats()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
			Valuer: f.statIncomingRate,
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
			Valuer: f.statProcessedRate,
		},
	)

	// Add stats
	f.BaseNode.AddStats(ss...)
}

// OutputCtx returns the output ctx
func (f *Forwarder) OutputCtx() Context {
	return f.outputCtx
}

// Connect implements the FrameHandlerConnector interface
func (f *Forwarder) Connect(h FrameHandler) {
	// Add handler
	f.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(f, h)
}

// Disconnect implements the FrameHandlerConnector interface
func (f *Forwarder) Disconnect(h FrameHandler) {
	// Delete handler
	f.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(f, h)
}

// Start starts the forwarder
func (f *Forwarder) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	f.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to stop the chan properly
		defer f.c.Stop()

		// Start chan
		f.c.Start(f.Context())
	})
}

// HandleFrame implements the FrameHandler interface
func (f *Forwarder) HandleFrame(p FrameHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	f.DoWhenUnclosed(func() {
		// Increment incoming rate
		f.statIncomingRate.Add(1)

		// Copy frame
		fm := f.p.get()
		if err := fm.Ref(p.Frame); err != nil {
			emitError(f, f.eh, err, "refing frame")
			return
		}

		// Add to chan
		f.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			f.DoWhenUnclosed(func() {
				// Handle pause
				defer f.HandlePause()

				// Make sure to close frame
				defer f.p.put(fm)

				// Increment processed rate
				f.statProcessedRate.Add(1)

				// Restamp
				if f.restamper != nil {
					f.restamper.Restamp(fm)
				}

				// Dispatch frame
				f.d.dispatch(fm, p.Descriptor)
			})
		})
	})
}
