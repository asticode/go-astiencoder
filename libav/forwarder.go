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
	c                *astikit.Chan
	d                *frameDispatcher
	outputCtx        Context
	restamper        FrameRestamper
	statIncomingRate *astikit.CounterRateStat
	statWorkRatio    *astikit.DurationPercentageStat
}

// ForwarderOptions represents forwarder options
type ForwarderOptions struct {
	Node      astiencoder.NodeOptions
	OutputCtx Context
	Restamper FrameRestamper
}

// NewForwarder creates a new forwarder
func NewForwarder(o ForwarderOptions, eh *astiencoder.EventHandler, c *astikit.Closer) (f *Forwarder) {
	// Extend node metadata
	count := atomic.AddUint64(&countForwarder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("forwarder_%d", count), fmt.Sprintf("Forwarder #%d", count), "Forwards", "forwarder")

	// Create forwarder
	f = &Forwarder{
		c: astikit.NewChan(astikit.ChanOptions{
			AddStrategy: astikit.ChanAddStrategyBlockWhenStarted,
			ProcessAll:  true,
		}),
		outputCtx:        o.OutputCtx,
		restamper:        o.Restamper,
		statIncomingRate: astikit.NewCounterRateStat(),
		statWorkRatio:    astikit.NewDurationPercentageStat(),
	}
	f.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(f), eh)
	f.d = newFrameDispatcher(f, eh, c)
	f.addStats()
	return
}

func (f *Forwarder) addStats() {
	// Add incoming rate
	f.Stater().AddStat(astikit.StatMetadata{
		Description: "Number of frames coming in per second",
		Label:       "Incoming rate",
		Unit:        "fps",
	}, f.statIncomingRate)

	// Add work ratio
	f.Stater().AddStat(astikit.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, f.statWorkRatio)

	// Add dispatcher stats
	f.d.addStats(f.Stater())

	// Add chan stats
	f.c.AddStats(f.Stater())
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
		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer f.d.wait()

		// Make sure to stop the chan properly
		defer f.c.Stop()

		// Start chan
		f.c.Start(f.Context())
	})
}

// HandleFrame implements the FrameHandler interface
func (f *Forwarder) HandleFrame(p *FrameHandlerPayload) {
	f.c.Add(func() {
		// Handle pause
		defer f.HandlePause()

		// Increment incoming rate
		f.statIncomingRate.Add(1)

		// Restamp
		if f.restamper != nil {
			f.statWorkRatio.Begin()
			f.restamper.Restamp(p.Frame)
			f.statWorkRatio.End()
		}

		// Dispatch frame
		f.d.dispatch(p.Frame, p.Descriptor)
	})
}
