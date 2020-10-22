package astilibav

import (
	"fmt"
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avutil"
)

// FrameHandler represents a node that can handle a frame
type FrameHandler interface {
	astiencoder.Node
	HandleFrame(p *FrameHandlerPayload)
}

// FrameHandlerConnector represents an object that can connect/disconnect with a frame handler
type FrameHandlerConnector interface {
	Connect(next FrameHandler)
	Disconnect(next FrameHandler)
}

// FrameHandlerPayload represents a FrameHandler payload
type FrameHandlerPayload struct {
	Descriptor Descriptor
	Frame      *avutil.Frame
	Node       astiencoder.Node
	p          *framePool
}

// Close closes the payload
func (p *FrameHandlerPayload) Close() {
	p.p.put(p.Frame)
}

type frameDispatcher struct {
	eh               *astiencoder.EventHandler
	hs               map[string]FrameHandler
	m                *sync.Mutex // Locks hs
	n                astiencoder.Node
	p                *framePool
	statOutgoingRate *astikit.CounterRateStat
}

func newFrameDispatcher(n astiencoder.Node, eh *astiencoder.EventHandler, c *astikit.Closer) *frameDispatcher {
	return &frameDispatcher{
		eh:               eh,
		hs:               make(map[string]FrameHandler),
		m:                &sync.Mutex{},
		n:                n,
		p:                newFramePool(c),
		statOutgoingRate: astikit.NewCounterRateStat(),
	}
}

func (d *frameDispatcher) addHandler(h FrameHandler) {
	d.m.Lock()
	defer d.m.Unlock()
	d.hs[h.Metadata().Name] = h
}

func (d *frameDispatcher) delHandler(h FrameHandler) {
	d.m.Lock()
	defer d.m.Unlock()
	delete(d.hs, h.Metadata().Name)
}

func (d *frameDispatcher) newFrameHandlerPayload(f *avutil.Frame, descriptor Descriptor) (p *FrameHandlerPayload, err error) {
	// Create payload
	p = &FrameHandlerPayload{
		Descriptor: descriptor,
		Node:       d.n,
		p:          d.p,
	}

	// Copy frame
	p.Frame = d.p.get()
	if ret := avutil.AvFrameRef(p.Frame, f); ret < 0 {
		err = fmt.Errorf("astilibav: avutil.AvFrameRef failed: %w", NewAvError(ret))
		return
	}
	return
}

func (d *frameDispatcher) dispatch(f *avutil.Frame, descriptor Descriptor) {
	// Increment outgoing rate
	d.statOutgoingRate.Add(1)

	// Get handlers
	d.m.Lock()
	var hs []FrameHandler
	for _, h := range d.hs {
		hs = append(hs, h)
	}
	d.m.Unlock()

	// No handlers
	if len(hs) == 0 {
		return
	}

	// Loop through handlers
	for _, h := range hs {
		// Create payload
		p, err := d.newFrameHandlerPayload(f, descriptor)
		if err != nil {
			d.eh.Emit(astiencoder.EventError(d.n, fmt.Errorf("astilibav: creating frame handler payload failed: %w", err)))
			continue
		}

		// Handle frame
		h.HandleFrame(p)
	}
}

func (d *frameDispatcher) addStats(s *astikit.Stater) {
	// Add outgoing rate
	s.AddStat(astikit.StatMetadata{
		Description: "Number of frames going out per second",
		Label:       "Outgoing rate",
		Name:        StatNameOutgoingRate,
		Unit:        "fps",
	}, d.statOutgoingRate)
}

type framePool struct {
	c *astikit.Closer
	m *sync.Mutex
	p []*avutil.Frame
}

func newFramePool(c *astikit.Closer) *framePool {
	return &framePool{
		c: c,
		m: &sync.Mutex{},
	}
}

func (p *framePool) get() (f *avutil.Frame) {
	p.m.Lock()
	defer p.m.Unlock()
	if len(p.p) == 0 {
		f = avutil.AvFrameAlloc()
		p.c.Add(func() error {
			avutil.AvFrameFree(f)
			return nil
		})
		return
	}
	f = p.p[0]
	p.p = p.p[1:]
	return
}

func (p *framePool) put(f *avutil.Frame) {
	p.m.Lock()
	defer p.m.Unlock()
	avutil.AvFrameUnref(f)
	p.p = append(p.p, f)
}
