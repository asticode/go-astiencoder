package astilibav

import (
	"fmt"
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
)

// PktHandler represents a node that can handle a pkt
type PktHandler interface {
	astiencoder.Node
	HandlePkt(p *PktHandlerPayload)
}

// PktHandlerConnector represents an object that can connect/disconnect with a pkt handler
type PktHandlerConnector interface {
	Connect(next PktHandler)
	Disconnect(next PktHandler)
}

// PktHandlerPayload represents a PktHandler payload
type PktHandlerPayload struct {
	Descriptor Descriptor
	p          *pktPool
	Pkt        *avcodec.Packet
}

// Close closes the payload
func (p *PktHandlerPayload) Close() {
	p.p.put(p.Pkt)
}

type pktDispatcher struct {
	eh               *astiencoder.EventHandler
	hs               map[string]PktHandler
	m                *sync.Mutex
	n                astiencoder.Node
	p                *pktPool
	statOutgoingRate *astikit.CounterRateStat
}

func newPktDispatcher(n astiencoder.Node, eh *astiencoder.EventHandler, c *astikit.Closer) *pktDispatcher {
	return &pktDispatcher{
		eh:               eh,
		hs:               make(map[string]PktHandler),
		m:                &sync.Mutex{},
		n:                n,
		p:                newPktPool(c),
		statOutgoingRate: astikit.NewCounterRateStat(),
	}
}

func (d *pktDispatcher) addHandler(h PktHandler) {
	d.m.Lock()
	defer d.m.Unlock()
	d.hs[h.Metadata().Name] = h
}

func (d *pktDispatcher) delHandler(h PktHandler) {
	d.m.Lock()
	defer d.m.Unlock()
	delete(d.hs, h.Metadata().Name)
}

func (d *pktDispatcher) newPktHandlerPayload(pkt *avcodec.Packet, descriptor Descriptor) (p *PktHandlerPayload, err error) {
	// Create payload
	p = &PktHandlerPayload{
		Descriptor: descriptor,
		p:          d.p,
	}

	// Copy pkt
	p.Pkt = d.p.get()
	if ret := p.Pkt.AvPacketRef(pkt); ret < 0 {
		err = fmt.Errorf("astilibav: AvPacketRef failed: %w", NewAvError(ret))
		return
	}
	return
}

func (d *pktDispatcher) dispatch(pkt *avcodec.Packet, descriptor Descriptor) {
	// Increment outgoing rate
	d.statOutgoingRate.Add(1)

	// Get handlers
	d.m.Lock()
	var hs []PktHandler
	for _, h := range d.hs {
		v, ok := h.(PktCond)
		if !ok || v.UsePkt(pkt) {
			hs = append(hs, h)
		}
	}
	d.m.Unlock()

	// No handlers
	if len(hs) == 0 {
		return
	}

	// Loop through handlers
	for _, h := range hs {
		// Create payload
		p, err := d.newPktHandlerPayload(pkt, descriptor)
		if err != nil {
			d.eh.Emit(astiencoder.EventError(d.n, fmt.Errorf("astilibav: creating pkt handler payload failed: %w", err)))
			continue
		}

		// Handle pkt
		h.HandlePkt(p)
	}
}

func (d *pktDispatcher) addStats(s *astikit.Stater) {
	// Add outgoing rate
	s.AddStat(astikit.StatMetadata{
		Description: "Number of packets going out per second",
		Label:       "Outgoing rate",
		Name:        StatNameOutgoingRate,
		Unit:        "pps",
	}, d.statOutgoingRate)
}

// PktCond represents an object that can decide whether to use a pkt
type PktCond interface {
	UsePkt(pkt *avcodec.Packet) bool
}

type pktCond struct {
	PktHandler
	i *avformat.Stream
}

func newPktCond(i *avformat.Stream, h PktHandler) *pktCond {
	return &pktCond{
		i:          i,
		PktHandler: h,
	}
}

// Metadata implements the NodeDescriptor interface
func (c *pktCond) Metadata() astiencoder.NodeMetadata {
	m := c.PktHandler.Metadata()
	m.Name = fmt.Sprintf("%s_%d", c.PktHandler.Metadata().Name, c.i.Index())
	return m
}

// UsePkt implements the PktCond interface
func (c *pktCond) UsePkt(pkt *avcodec.Packet) bool {
	return pkt.StreamIndex() == c.i.Index()
}

type pktPool struct {
	c *astikit.Closer
	m *sync.Mutex
	p []*avcodec.Packet
}

func newPktPool(c *astikit.Closer) *pktPool {
	return &pktPool{
		c: c,
		m: &sync.Mutex{},
	}
}

func (p *pktPool) get() (pkt *avcodec.Packet) {
	p.m.Lock()
	defer p.m.Unlock()
	if len(p.p) == 0 {
		pkt = avcodec.AvPacketAlloc()
		p.c.Add(func() error {
			avcodec.AvPacketFree(pkt)
			return nil
		})
		return
	}
	pkt = p.p[0]
	p.p = p.p[1:]
	return
}

func (p *pktPool) put(pkt *avcodec.Packet) {
	p.m.Lock()
	defer p.m.Unlock()
	pkt.AvPacketUnref()
	p.p = append(p.p, pkt)
}
