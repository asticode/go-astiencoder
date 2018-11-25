package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"

	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
)

var countSwitcher uint64

// Switcher represents an object capable of switching between sources
type Switcher struct {
	*astiencoder.BaseNode
	d                *frameDispatcher
	e                *astiencoder.EventEmitter
	m                *sync.Mutex
	q                *astisync.CtxQueue
	restamper        FrameRestamper
	rs               map[astiencoder.Node]bool
	statIncomingRate *astistat.IncrementStat
	statWorkRatio    *astistat.DurationRatioStat
}

// SwitcherOptions represents switcher options
type SwitcherOptions struct {
	Restamper FrameRestamper
}

// NewSwitcher creates a new switcher
func NewSwitcher(o SwitcherOptions, e *astiencoder.EventEmitter, c astiencoder.CloseFuncAdder) (s *Switcher) {
	count := atomic.AddUint64(&countSwitcher, uint64(1))
	s = &Switcher{
		BaseNode: astiencoder.NewBaseNode(e, astiencoder.NodeMetadata{
			Description: "Switcher",
			Label:       fmt.Sprintf("Switcher #%d", count),
			Name:        fmt.Sprintf("switcher_%d", count),
		}),
		m:                &sync.Mutex{},
		q:                astisync.NewCtxQueue(),
		restamper:        o.Restamper,
		rs:               make(map[astiencoder.Node]bool),
		statIncomingRate: astistat.NewIncrementStat(),
		statWorkRatio:    astistat.NewDurationRatioStat(),
	}
	s.d = newFrameDispatcher(s, e, c)
	s.addStats()
	return
}

func (s *Switcher) addStats() {
	// Add incoming rate
	s.Stater().AddStat(astistat.StatMetadata{
		Description: "Number of frames coming in per second",
		Label:       "Incoming rate",
		Unit:        "fps",
	}, s.statIncomingRate)

	// Add work ratio
	s.Stater().AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent doing some actual work",
		Label:       "Work ratio",
		Unit:        "%",
	}, s.statWorkRatio)

	// Add dispatcher stats
	s.d.addStats(s.Stater())

	// Add queue stats
	s.q.AddStats(s.Stater())
}

// SetRule sets a rule
func (s *Switcher) SetRule(n astiencoder.Node, v bool) {
	s.m.Lock()
	defer s.m.Unlock()
	s.rs[n] = v
}

// SetRules sets all rules
func (s *Switcher) SetRules(rs map[astiencoder.Node]bool) {
	s.m.Lock()
	defer s.m.Unlock()
	for k, v := range rs {
		s.rs[k] = v
	}
}

// Connect implements the FrameHandlerConnector interface
func (s *Switcher) Connect(h FrameHandler) {
	// Add handler
	s.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(s, h)
}

// Disconnect implements the FrameHandlerConnector interface
func (s *Switcher) Disconnect(h FrameHandler) {
	// Delete handler
	s.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(s, h)
}

// Start starts the filterer
func (s *Switcher) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	s.BaseNode.Start(ctx, t, func(t *astiworker.Task) {
		// Handle context
		go s.q.HandleCtx(s.Context())

		// Make sure to wait for all dispatcher subprocesses to be done so that they are properly closed
		defer s.d.wait()

		// Make sure to stop the queue properly
		defer s.q.Stop()

		// Start queue
		s.q.Start(func(dp interface{}) {
			// Handle pause
			defer s.HandlePause()

			// Assert payload
			p := dp.(*FrameHandlerPayload)

			// Increment incoming rate
			s.statIncomingRate.Add(1)

			// Check whether the frame should be forwarded
			s.statWorkRatio.Add(true)
			s.m.Lock()
			if v, ok := s.rs[p.Node]; !ok || !v {
				s.m.Unlock()
				return
			}
			s.m.Unlock()

			// Restamp frame
			if s.restamper != nil {
				s.restamper.Restamp(p.Frame, true)
			}

			// Dispatch frame
			s.d.dispatch(p.Frame, p.Descriptor)
		})
	})
}

// HandleFrame implements the FrameHandler interface
func (s *Switcher) HandleFrame(p *FrameHandlerPayload) {
	s.q.Send(p)
}
