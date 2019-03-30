package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	astiencoder "github.com/asticode/go-astiencoder"
	astidefer "github.com/asticode/go-astitools/defer"
	astistat "github.com/asticode/go-astitools/stat"
	astisync "github.com/asticode/go-astitools/sync"
	astiworker "github.com/asticode/go-astitools/worker"
)

var countForwarder uint64

// Forwarder represents an object capable of forwarding frames
type Forwarder struct {
	*astiencoder.BaseNode
	d                *frameDispatcher
	q                *astisync.CtxQueue
	restamper        FrameRestamper
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// ForwarderOptions represents forwarder options
type ForwarderOptions struct {
	Node      astiencoder.NodeOptions
	Restamper FrameRestamper
}

// NewForwarder creates a new forwarder
func NewForwarder(o ForwarderOptions, eh *astiencoder.EventHandler, c *astidefer.Closer) (f *Forwarder) {
	// Extend node metadata
	count := atomic.AddUint64(&countForwarder, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("forwarder_%d", count), fmt.Sprintf("Forwarder #%d", count), "Forwards")

	// Create forwarder
	f = &Forwarder{
		q:                astisync.NewCtxQueue(),
		restamper:        o.Restamper,
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	f.BaseNode = astiencoder.NewBaseNode(o.Node, astiencoder.NewEventGeneratorNode(f), eh)
	f.d = newFrameDispatcher(f, eh, c)
	f.addStats()
	return
}

func (f *Forwarder) addStats() {
	// Add incoming rate
	f.Stater().AddStat(astistat.StatMetadata{
		Description: "Number of frames coming in per second",
		Label:       "Incoming rate",
		Unit:        "fps",
	}, f.statIncomingRate)

	// Add work ratio
	f.Stater().AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, f.statWorkRatio)

	// Add dispatcher stats
	f.d.addStats(f.Stater())

	// Add queue stats
	f.q.AddStats(f.Stater())
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
	f.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go f.q.HandleCtx(f.Context())

		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer f.d.wait()

		// Make sure to stop the queue properly
		defer f.q.Stop()

		// Start queue
		f.q.Start(func(dp interface{}) {
			// Handle pause
			defer f.HandlePause()

			// Assert payload
			p := dp.(*FrameHandlerPayload)

			// Increment incoming rate
			f.statIncomingRate.Add(1)

			// Restamp
			if f.restamper != nil {
				f.statWorkRatio.Add(true)
				f.restamper.Restamp(p.Frame)
				f.statWorkRatio.Done(true)
			}

			// Dispatch frame
			f.d.dispatch(p.Frame, p.Descriptor)
		})
	})
}

// HandleFrame implements the FrameHandler interface
func (f *Forwarder) HandleFrame(p *FrameHandlerPayload) {
	f.q.Send(p)
}
