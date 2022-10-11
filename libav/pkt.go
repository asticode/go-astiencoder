package astilibav

import (
	"fmt"
	"sync"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

// PktHandler represents a node that can handle a pkt
type PktHandler interface {
	astiencoder.Node
	HandlePkt(p PktHandlerPayload)
}

// PktHandlerConnector represents an object that can connect/disconnect with a pkt handler
type PktHandlerConnector interface {
	Connect(next PktHandler)
	Disconnect(next PktHandler)
}

// PktHandlerPayload represents a PktHandler payload
type PktHandlerPayload struct {
	Descriptor Descriptor
	Node       astiencoder.Node
	Pkt        *astiav.Packet
}

type pktDispatcher struct {
	eh               *astiencoder.EventHandler
	hs               map[string]PktHandler
	m                *sync.Mutex
	n                astiencoder.Node
	statOutgoingRate *astikit.CounterRateStat
}

func newPktDispatcher(n astiencoder.Node, eh *astiencoder.EventHandler) *pktDispatcher {
	return &pktDispatcher{
		eh:               eh,
		hs:               make(map[string]PktHandler),
		m:                &sync.Mutex{},
		n:                n,
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

func (d *pktDispatcher) dispatch(pkt *astiav.Packet, descriptor Descriptor) {
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
		// Handle pkt
		h.HandlePkt(PktHandlerPayload{
			Descriptor: descriptor,
			Node:       d.n,
			Pkt:        pkt,
		})
	}
}

func (d *pktDispatcher) stats() []astikit.StatOptions {
	return []astikit.StatOptions{
		{
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets going out per second",
				Label:       "Outgoing rate",
				Name:        StatNameOutgoingRate,
				Unit:        "pps",
			},
			Valuer: d.statOutgoingRate,
		},
	}
}

// PktCond represents an object that can decide whether to use a pkt
type PktCond interface {
	UsePkt(pkt *astiav.Packet) bool
}

type pktCond struct {
	PktHandler
	i *Stream
}

func newPktCond(i *Stream, h PktHandler) *pktCond {
	return &pktCond{
		i:          i,
		PktHandler: h,
	}
}

// Metadata implements the NodeDescriptor interface
func (c *pktCond) Metadata() astiencoder.NodeMetadata {
	m := c.PktHandler.Metadata()
	m.Name = fmt.Sprintf("%s_%d", c.PktHandler.Metadata().Name, c.i.Index)
	return m
}

// UsePkt implements the PktCond interface
func (c *pktCond) UsePkt(pkt *astiav.Packet) bool {
	return pkt.StreamIndex() == c.i.Index
}

type pktPool struct {
	c                  astiencoder.Closer
	m                  *sync.Mutex
	p                  []*astiav.Packet
	statAllocatedCount *astikit.CounterStat
}

func newPktPool(c astiencoder.Closer) *pktPool {
	return &pktPool{
		c:                  c,
		m:                  &sync.Mutex{},
		statAllocatedCount: astikit.NewCounterStat(),
	}
}

func (p *pktPool) get() (pkt *astiav.Packet) {
	p.m.Lock()
	defer p.m.Unlock()
	if len(p.p) == 0 {
		pkt = astiav.AllocPacket()
		p.statAllocatedCount.Add(1)
		p.c.AddClose(pkt.Free)
		return
	}
	pkt = p.p[0]
	p.p = p.p[1:]
	return
}

func (p *pktPool) put(pkt *astiav.Packet) {
	p.m.Lock()
	defer p.m.Unlock()
	pkt.Unref()
	p.p = append(p.p, pkt)
}

func (p *pktPool) stats() []astikit.StatOptions {
	return []astikit.StatOptions{
		{
			Metadata: &astikit.StatMetadata{
				Description: "Number of allocated packets",
				Label:       "Allocated packets",
				Name:        StatNameAllocatedPackets,
				Unit:        "p",
			},
			Valuer: p.statAllocatedCount,
		},
	}
}
