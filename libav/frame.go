package astilibav

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
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
	Frame      *astiav.Frame
	Node       astiencoder.Node
}

type frameDispatcher struct {
	eh                   *astiencoder.EventHandler
	hs                   map[string]FrameHandler
	m                    *sync.Mutex // Locks hs
	n                    astiencoder.Node
	statFramesDispatched uint64
}

func newFrameDispatcher(n astiencoder.Node, eh *astiencoder.EventHandler) *frameDispatcher {
	return &frameDispatcher{
		eh: eh,
		hs: make(map[string]FrameHandler),
		m:  &sync.Mutex{},
		n:  n,
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

func (d *frameDispatcher) dispatch(f *astiav.Frame, descriptor Descriptor) {
	// Increment dispatched frames
	atomic.AddUint64(&d.statFramesDispatched, 1)

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

type frameDispatcherStats struct {
	framesDispatched uint64
}

func (d *frameDispatcher) stats() frameDispatcherStats {
	return frameDispatcherStats{framesDispatched: atomic.LoadUint64(&d.statFramesDispatched)}
}

func (d *frameDispatcher) statOptions() []astikit.StatOptions {
	return []astikit.StatOptions{
		{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames going out per second",
				Label:       "Outgoing rate",
				Name:        StatNameOutgoingRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&d.statFramesDispatched),
		},
	}
}

type framePool struct {
	c                   astiencoder.Closer
	m                   *sync.Mutex
	p                   []*astiav.Frame
	statFramesAllocated uint64
}

func newFramePool(c astiencoder.Closer) *framePool {
	return &framePool{
		c: c,
		m: &sync.Mutex{},
	}
}

func (p *framePool) get() (f *astiav.Frame) {
	p.m.Lock()
	defer p.m.Unlock()
	if len(p.p) == 0 {
		f = astiav.AllocFrame()
		atomic.AddUint64(&p.statFramesAllocated, 1)
		p.c.AddClose(f.Free)
		return
	}
	f = p.p[0]
	p.p = p.p[1:]
	return
}

func (p *framePool) put(f *astiav.Frame) {
	p.m.Lock()
	defer p.m.Unlock()
	f.Unref()
	p.p = append(p.p, f)
}

type framePoolStats struct {
	framesAllocated uint64
}

func (p *framePool) stats() framePoolStats {
	return framePoolStats{framesAllocated: atomic.LoadUint64(&p.statFramesAllocated)}
}

func (p *framePool) statOptions() []astikit.StatOptions {
	return []astikit.StatOptions{
		{
			Metadata: &astikit.StatMetadata{
				Description: "Number of allocated frames",
				Label:       "Allocated frames",
				Name:        StatNameAllocatedFrames,
				Unit:        "f",
			},
			Valuer: astikit.StatValuerFunc(func(d time.Duration) interface{} { return atomic.LoadUint64(&p.statFramesAllocated) }),
		},
	}
}
