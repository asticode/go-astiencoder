package astilibav

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countSwitcher uint64

// Switcher represents an object capable of switching between nodes while enforcing constant PTS delta
type Switcher struct {
	*astiencoder.BaseNode
	c                   *astikit.Chan
	currentPTS          *int64
	d                   *frameDispatcher
	eh                  *astiencoder.EventHandler
	frameTimeout        time.Duration
	ff                  *FrameFiller
	frameDuration       int64
	is                  []*switcherItem
	outputCtx           Context
	m                   *sync.Mutex
	p                   *framePool
	statFramesProcessed uint64
	statFramesReceived  uint64
}

type switcherFrame struct {
	at time.Time
	f  *astiav.Frame
}

func newSwitcherFrame(f *astiav.Frame) *switcherFrame {
	return &switcherFrame{
		at: time.Now(),
		f:  f,
	}
}

type switcherItem struct {
	fs     map[int64]*switcherFrame
	n      astiencoder.Node
	ptsMax *int64
	ptsMin *int64
}

func newSwitcherItem(n astiencoder.Node) *switcherItem {
	return &switcherItem{
		fs: map[int64]*switcherFrame{},
		n:  n,
	}
}

// SwitcherOptions represents switcher options
type SwitcherOptions struct {
	FrameDuration int64
	FrameTimeout  time.Duration
	Node          astiencoder.NodeOptions
	OutputCtx     Context
}

// NewSwitcher creates a new switcher
func NewSwitcher(o SwitcherOptions, eh *astiencoder.EventHandler, c *astikit.Closer, st *astiencoder.Stater) (s *Switcher, err error) {
	// Extend node metadata
	count := atomic.AddUint64(&countSwitcher, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("switcher_%d", count), fmt.Sprintf("Switcher #%d", count), "Switches", "switcher")

	// Create switcher
	s = &Switcher{
		c:             astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:            eh,
		frameDuration: o.FrameDuration,
		frameTimeout:  o.FrameTimeout,
		m:             &sync.Mutex{},
		outputCtx:     o.OutputCtx,
	}

	// Create base node
	s.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, st, s, astiencoder.EventTypeToNodeEventName)

	// Create frame filler
	s.ff = NewFrameFiller(s.NewChildCloser(), eh, s)

	// Adapt frame filler
	switch s.outputCtx.MediaType {
	case astiav.MediaTypeAudio:
		if _, err = s.ff.WithFallbackFrame(EmptyAudioFrameAdapter(
			s.outputCtx.FrameSize,
			s.outputCtx.SampleRate,
			s.outputCtx.ChannelLayout,
			s.outputCtx.SampleFormat,
		)); err != nil {
			err = fmt.Errorf("astilibav: adapting frame filler failed: %w", err)
			return
		}
	case astiav.MediaTypeVideo:
		s.ff.WithPreviousFrame()
	default:
		err = fmt.Errorf("astilibav: invalid %s media type", s.outputCtx.MediaType)
		return
	}

	// Create frame pool
	s.p = newFramePool(s.NewChildCloser())

	// Create frame dispatcher
	s.d = newFrameDispatcher(s, eh)

	// Add stat options
	s.addStatOptions()
	return
}

type SwitcherStats struct {
	FramesAllocated  uint64
	FramesDispatched uint64
	FramesProcessed  uint64
	FramesReceived   uint64
	WorkDuration     time.Duration
}

func (s *Switcher) Stats() SwitcherStats {
	return SwitcherStats{
		FramesAllocated:  s.p.stats().framesAllocated,
		FramesDispatched: s.d.stats().framesDispatched,
		FramesProcessed:  atomic.LoadUint64(&s.statFramesProcessed),
		FramesReceived:   atomic.LoadUint64(&s.statFramesReceived),
		WorkDuration:     s.c.Stats().WorkDuration,
	}
}

func (s *Switcher) addStatOptions() {
	// Get stats
	ss := s.c.StatOptions()
	ss = append(ss, s.d.statOptions()...)
	ss = append(ss, s.p.statOptions()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&s.statFramesReceived),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&s.statFramesProcessed),
		},
	)

	// Add stats
	s.BaseNode.AddStats(ss...)
}

// OutputCtx returns the output ctx
func (s *Switcher) OutputCtx() Context {
	return s.outputCtx
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

// Switch switches the source
func (s *Switcher) Switch(n astiencoder.Node) {
	// Lock
	s.m.Lock()
	defer s.m.Unlock()

	// Remove last item if it was never used
	if len(s.is) > 0 && s.is[len(s.is)-1].ptsMin == nil {
		s.is = s.is[:len(s.is)-1]
	}

	// Do nothing if last item has the same node
	if len(s.is) > 0 && s.is[len(s.is)-1].n == n {
		return
	}

	// Append
	s.is = append(s.is, newSwitcherItem(n))
}

// Start starts the switcher
func (s *Switcher) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	s.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to stop the chan properly
		defer s.c.Stop()

		// Start chan
		s.c.Start(s.Context())
	})
}

// HandleFrame implements the FrameHandler interface
func (s *Switcher) HandleFrame(p FrameHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	s.DoWhenUnclosed(func() {
		// Increment received frames
		atomic.AddUint64(&s.statFramesReceived, 1)

		// Copy frame
		f := s.p.get()
		if err := f.Ref(p.Frame); err != nil {
			emitError(s, s.eh, err, "refing frame")
			return
		}

		// Add to chan
		s.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			s.DoWhenUnclosed(func() {
				// Handle pause
				defer s.HandlePause()

				// Increment processed frames
				atomic.AddUint64(&s.statFramesProcessed, 1)

				// Lock
				s.m.Lock()
				defer s.m.Unlock()

				// Add frame
				if added := s.addUnsafe(f, p.Node); !added {
					s.p.put(f)
					return
				}

				// Pull frames
				ps, switched := s.pullUnsafe()

				// Make sure to close frames
				defer func() {
					for _, p := range ps {
						if !p.filled {
							s.p.put(p.f)
						}
					}
				}()

				// Dispatch frames
				for _, p := range ps {
					// Put in frame filler
					if !p.filled {
						s.ff.Put(p.f, nil)
					}

					// Dispatch
					s.d.dispatch(p.f, s.outputCtx.Descriptor())
				}

				// Loop through switched nodes
				for _, n := range switched {
					// Emit event
					s.eh.Emit(astiencoder.Event{
						Name:    EventNameSwitcherSwitchedOut,
						Payload: n,
						Target:  s,
					})
				}

				// Clean
				s.cleanUnsafe()
			})
		})
	})
}

func (s *Switcher) addUnsafe(f *astiav.Frame, n astiencoder.Node) (added bool) {
	// Loop through items
	for _, i := range s.is {
		// Invalid node
		if i.n != n {
			continue
		}

		// Valid pts
		if i.ptsMin != nil && *i.ptsMin <= f.Pts() && (i.ptsMax == nil || *i.ptsMax > f.Pts()) {
			i.fs[f.Pts()] = newSwitcherFrame(f)
			added = true
			break
		}
	}

	// Frame has not been added but last item, being empty with the proper node, is a fit
	if !added && len(s.is) > 0 && s.is[len(s.is)-1].n == n && s.is[len(s.is)-1].ptsMin == nil {
		// Get min pts
		ptsMin := f.Pts()
		if s.currentPTS != nil && *s.currentPTS >= f.Pts() {
			ptsMin = *s.currentPTS + s.frameDuration
		}

		// Update last item
		s.is[len(s.is)-1].ptsMin = &ptsMin

		// Add frame
		if f.Pts() >= ptsMin {
			s.is[len(s.is)-1].fs[f.Pts()] = newSwitcherFrame(f)
			added = true
		}
	}

	// Frame has been added
	if added {
		// Loop through items except the last one
		for idx := 0; idx < len(s.is)-1; idx++ {
			// Get item
			i := s.is[idx]

			// Item needs to be closed
			if i.ptsMax == nil && s.is[idx+1].ptsMin != nil {
				// Close item
				i.ptsMax = astikit.Int64Ptr(*s.is[idx+1].ptsMin)

				// Remove frames outside of interval
				for pts := range i.fs {
					// Invalid pts
					if *i.ptsMin > pts || *i.ptsMax <= pts {
						// Remove pts
						delete(i.fs, pts)
					}
				}
			}
		}
	}
	return
}

type switcherPulledFrame struct {
	f      *astiav.Frame
	filled bool
}

func (s *Switcher) pullUnsafe() (ps []*switcherPulledFrame, switched []astiencoder.Node) {
	// Loop
	for {
		// Get next pull
		p, sn := s.nextPullUnsafe()
		if p == nil {
			return
		}

		// Update
		ps = append(ps, p)
		if sn != nil {
			switched = append(switched, sn)
		}
	}
}

func (s *Switcher) nextPullUnsafe() (p *switcherPulledFrame, switched astiencoder.Node) {
	// Make sure to update current pts
	defer func() {
		if p != nil {
			s.currentPTS = astikit.Int64Ptr(p.f.Pts())
		}
	}()

	// No current pts
	if s.currentPTS == nil {
		i := s.is[0]
		p = &switcherPulledFrame{f: i.fs[*i.ptsMin].f}
		delete(i.fs, *i.ptsMin)
		switched = i.n
		return
	}

	// Get next pts
	nextPTS := *s.currentPTS + s.frameDuration

	// Loop through items
	for _, i := range s.is {
		// Invalid pts
		if i.ptsMin == nil || *i.ptsMin > nextPTS || (i.ptsMax != nil && *i.ptsMax <= nextPTS) {
			continue
		}

		// Get frame
		var filled bool
		sf, ok := i.fs[nextPTS]

		// Frame doesn't exist but there are already later frames available, we need to fill
		var f *astiav.Frame
		if !ok && len(i.fs) > 0 {
			// Fill frame
			if f, _ = s.ff.Get(); f != nil {
				filled = true
				f.SetPts(nextPTS)
			}
		} else if ok {
			f = sf.f
			delete(i.fs, nextPTS)
		}

		// Frame has been pulled
		if f != nil {
			// Create pulled frame
			p = &switcherPulledFrame{
				f:      f,
				filled: filled,
			}

			// Update switched
			if *i.ptsMin == nextPTS {
				switched = i.n
			}
			return
		}
	}

	// No frame timeout
	if s.frameTimeout <= 0 {
		return
	}

	// No valid frame was found but we still need to check whether some frame has timed out
	// Loop through items
	for _, i := range s.is {
		// Loop through frames
		for _, sf := range i.fs {
			// Frame has timed out
			if time.Since(sf.at) >= s.frameTimeout {
				// Fill frame
				f, _ := s.ff.Get()
				if f != nil {
					// Restamp
					f.SetPts(nextPTS)

					// Create pulled frame
					p = &switcherPulledFrame{
						f:      f,
						filled: true,
					}
					return
				}
			}
		}
	}
	return
}

func (s *Switcher) cleanUnsafe() {
	// Remove useless items
	for idx := 0; idx < len(s.is); idx++ {
		// Get item
		i := s.is[idx]

		// Item should be removed
		if i.ptsMax != nil && s.currentPTS != nil && *i.ptsMax <= *s.currentPTS+s.frameDuration {
			// Close remaining frames
			for _, f := range i.fs {
				s.p.put(f.f)
			}

			// Remove item
			s.is = append(s.is[:idx], s.is[idx+1:]...)
			idx--
		}
	}
}
