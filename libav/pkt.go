package astilibav

import (
	"fmt"
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
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
	Pkt        *avcodec.Packet
}

type pktDispatcher struct {
	eh               *astiencoder.EventHandler
	hs               map[string]PktHandler
	m                *sync.Mutex
	n                astiencoder.Node
	p                *pktPool
	statOutgoingRate *astikit.CounterRateStat
}

func newPktDispatcher(n astiencoder.Node, eh *astiencoder.EventHandler, p *pktPool) *pktDispatcher {
	return &pktDispatcher{
		eh:               eh,
		hs:               make(map[string]PktHandler),
		m:                &sync.Mutex{},
		n:                n,
		p:                p,
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
			Handler: d.statOutgoingRate,
			Metadata: &astikit.StatMetadata{
				Description: "Number of packets going out per second",
				Label:       "Outgoing rate",
				Name:        StatNameOutgoingRate,
				Unit:        "pps",
			},
		},
	}
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

func pktDuration(pkt *avcodec.Packet, ctx Context) int64 {
	switch ctx.CodecType {
	case avutil.AVMEDIA_TYPE_AUDIO:
		// Get skip samples side data
		sd := pkt.AvPacketGetSideData(avcodec.AV_PKT_DATA_SKIP_SAMPLES, nil)
		if sd == nil {
			return pkt.Duration()
		}

		// Substract number of samples
		skipStart, skipEnd := avutil.AV_RL32(sd, 0), avutil.AV_RL32(sd, 4)
		return pkt.Duration() - avutil.AvRescaleQ(int64(float64(skipStart+skipEnd)/float64(ctx.SampleRate)*1e9), nanosecondRational, ctx.TimeBase)
	default:
		return pkt.Duration()
	}
}
