package astilibav

import (
	"fmt"
	"sync"
	"time"

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
	c astiencoder.Closer
	m *sync.Mutex
	p []*astiav.Packet
}

func newPktPool(c astiencoder.Closer) *pktPool {
	return &pktPool{
		c: c,
		m: &sync.Mutex{},
	}
}

func (p *pktPool) get() (pkt *astiav.Packet) {
	p.m.Lock()
	defer p.m.Unlock()
	if len(p.p) == 0 {
		pkt = astiav.AllocPacket()
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

type pktDurationer struct {
	ctx              Context
	previousDTS      *int64
	previousDuration int64
	previousSkipped  int64
	skippedRemainder time.Duration
}

func newPktDurationer(ctx Context) *pktDurationer {
	return &pktDurationer{
		ctx: ctx,
	}
}

func (pd *pktDurationer) handlePkt(pkt *astiav.Packet) (previousDuration int64) {
	// Make sure to update previous attributes
	defer func() {
		if pkt != nil {
			pd.previousDTS = astikit.Int64Ptr(pkt.Dts())
			pd.previousDuration = pkt.Duration()
			pd.previousSkipped = pd.skipped(pkt)
		}
	}()

	// No previous DTS
	if pd.previousDTS == nil {
		return
	}

	// Get duration
	// Use DTS delta since we can't trust pkt.Duration()
	return pkt.Dts() - *pd.previousDTS - pd.previousSkipped
}

func (pd *pktDurationer) flush() (lastDuration int64) {
	lastDuration = pd.previousDuration - pd.previousSkipped
	pd.previousDTS = nil
	pd.previousDuration = 0
	pd.previousSkipped = 0
	return
}

func (pd *pktDurationer) skipped(pkt *astiav.Packet) (i int64) {
	// Switch on media type
	var d time.Duration
	switch pd.ctx.MediaType {
	case astiav.MediaTypeAudio:
		// Get skip samples side data
		sd := pkt.SideData(astiav.PacketSideDataTypeSkipSamples)
		if sd == nil {
			return
		}

		// Get skipped duration
		skipStart, skipEnd := astiav.RL32WithOffset(sd, 0), astiav.RL32WithOffset(sd, 4)
		d = time.Duration(float64(skipStart+skipEnd) / float64(pd.ctx.SampleRate) * 1e9)
	default:
		return
	}

	// Add remainder
	d += pd.skippedRemainder

	// Convert duration to timebase
	i, pd.skippedRemainder = durationToTimeBase(d, pd.ctx.TimeBase)
	return
}
