package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countPktPiper uint64

// PktPiper represents an object capable of piping pkts
type PktPiper struct {
	*astiencoder.BaseNode
	c                 *astikit.Chan
	d                 *pktDispatcher
	ds                Descriptor
	eh                *astiencoder.EventHandler
	outputCtx         Context
	p                 *pktPool
	restamper         PktRestamper
	statIncomingRate  *astikit.CounterRateStat
	statProcessedRate *astikit.CounterRateStat
}

// PktPiperOptions represents pkt piper options
type PktPiperOptions struct {
	Node      astiencoder.NodeOptions
	OutputCtx Context
	Restamper PktRestamper
}

// NewPktPiper creates a new pkt piper
func NewPktPiper(o PktPiperOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (p *PktPiper) {
	// Extend node metadata
	count := atomic.AddUint64(&countPktPiper, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("pkt_piper_%d", count), fmt.Sprintf("Pkt piper #%d", count), "Pipes pkts", "pkt piper")

	// Create pkt piper
	p = &PktPiper{
		c:                 astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		ds:                NewDescriptor(o.OutputCtx.TimeBase),
		eh:                eh,
		outputCtx:         o.OutputCtx,
		restamper:         o.Restamper,
		statIncomingRate:  astikit.NewCounterRateStat(),
		statProcessedRate: astikit.NewCounterRateStat(),
	}

	// Create base node
	p.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, p, astiencoder.EventTypeToNodeEventName)

	// Create pkt pool
	p.p = newPktPool(p)

	// Create pkt dispatcher
	p.d = newPktDispatcher(p, eh)

	// Add stats
	p.addStats()
	return
}

func (p *PktPiper) addStats() {
	// Get stats
	ss := p.c.Stats()
	ss = append(ss, p.d.stats()...)
	ss = append(ss, p.p.stats()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of pkts coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
			Valuer: p.statIncomingRate,
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of pkts processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
			Valuer: p.statProcessedRate,
		},
	)

	// Add stats
	p.BaseNode.AddStats(ss...)
}

// OutputCtx returns the output ctx
func (p *PktPiper) OutputCtx() Context {
	return p.outputCtx
}

// Connect implements the PktHandlerConnector interface
func (p *PktPiper) Connect(h PktHandler) {
	// Add handler
	p.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(p, h)
}

// Disconnect implements the PktHandlerConnector interface
func (p *PktPiper) Disconnect(h PktHandler) {
	// Delete handler
	p.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(p, h)
}

// Start starts the piper
func (p *PktPiper) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	p.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to stop the chan properly
		defer p.c.Stop()

		// Start chan
		p.c.Start(p.Context())
	})
}

func (p *PktPiper) Pipe(data []byte) {
	// Everything executed outside the main loop should be protected from the closer
	p.DoWhenUnclosed(func() {
		// Increment incoming rate
		p.statIncomingRate.Add(1)

		// Get pkt
		pkt := p.p.get()

		// Load data
		if err := pkt.FromData(data); err != nil {
			emitError(p, p.eh, err, "loading data in packet")
			return
		}

		// Add to chan
		p.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			p.DoWhenUnclosed(func() {
				// Handle pause
				defer p.HandlePause()

				// Make sure to close pkt
				defer p.p.put(pkt)

				// Increment processed rate
				p.statProcessedRate.Add(1)

				// Restamp
				if p.restamper != nil {
					p.restamper.Restamp(pkt)
				}

				// Dispatch pkt
				p.d.dispatch(pkt, p.ds)
			})
		})
	})
}
