package astilibav

import (
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avutil"
)

// FrameHandler represents a node that can handle a frame
type FrameHandler interface {
	astiencoder.Node
	HandleFrame(p FrameHandlerPayload)
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
}

type frameDispatcher struct {
	eh               *astiencoder.EventHandler
	hs               map[string]FrameHandler
	m                *sync.Mutex // Locks hs
	n                astiencoder.Node
	p                *framePool
	statOutgoingRate *astikit.CounterRateStat
}

func newFrameDispatcher(n astiencoder.Node, eh *astiencoder.EventHandler, p *framePool) *frameDispatcher {
	return &frameDispatcher{
		eh:               eh,
		hs:               make(map[string]FrameHandler),
		m:                &sync.Mutex{},
		n:                n,
		p:                p,
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
		// Handle frame
		h.HandleFrame(FrameHandlerPayload{
			Descriptor: descriptor,
			Frame:      f,
			Node:       d.n,
		})
	}
}

func (d *frameDispatcher) stats() []astikit.StatOptions {
	return []astikit.StatOptions{
		{
			Handler: d.statOutgoingRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames going out per second",
				Label:       "Outgoing rate",
				Name:        StatNameOutgoingRate,
				Unit:        "fps",
			},
		},
	}
}

type framePool struct {
	c astiencoder.Closer
	m *sync.Mutex
	p []*avutil.Frame
}

func newFramePool(c astiencoder.Closer) *framePool {
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
		p.c.AddCloseFunc(func() error {
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
