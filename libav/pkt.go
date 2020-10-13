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
	Pkt        *avcodec.Packet
}

type pktDispatcher struct {
	hs               map[string]PktHandler
	m                *sync.Mutex
	p                *pktPool
	statDispatch     *astikit.DurationPercentageStat
	statOutgoingRate *astikit.CounterRateStat
	wg               *sync.WaitGroup
}

func newPktDispatcher(c *astikit.Closer) *pktDispatcher {
	return &pktDispatcher{
		hs:               make(map[string]PktHandler),
		m:                &sync.Mutex{},
		p:                newPktPool(c),
		statDispatch:     astikit.NewDurationPercentageStat(),
		statOutgoingRate: astikit.NewCounterRateStat(),
		wg:               &sync.WaitGroup{},
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
	// Copy handlers
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

	// Wait for all previous subprocesses to be done
	// In case a brave soul tries to update this logic so that several packet can be sent to handlers in parallel, bare
	// in mind that packets must be sent in order whereas sending packets in goroutines doesn't keep this order
	d.statDispatch.Begin()
	d.wait()
	d.statDispatch.End()

	// Increment outgoing rate
	d.statOutgoingRate.Add(1)

	// Add subprocesses
	d.wg.Add(len(hs))

	// Loop through handlers
	for _, h := range hs {
		// Copy pkt
		hPkt := d.p.get()
		hPkt.AvPacketRef(pkt)

		// Handle pkt
		go func(h PktHandler) {
			defer d.wg.Done()
			defer d.p.put(hPkt)
			h.HandlePkt(&PktHandlerPayload{
				Descriptor: descriptor,
				Pkt:        hPkt,
			})
		}(h)
	}
}

func (d *pktDispatcher) wait() {
	d.wg.Wait()
}

func (d *pktDispatcher) addStats(s *astikit.Stater) {
	// Add outgoing rate
	s.AddStat(astikit.StatMetadata{
		Description: "Number of packets going out per second",
		Label:       "Outgoing rate",
		Name:        StatNameOutgoingRate,
		Unit:        "pps",
	}, d.statOutgoingRate)

	// Add wait time
	s.AddStat(astikit.StatMetadata{
		Description: "Percentage of time spent waiting for all previous subprocesses to be done",
		Label:       "Dispatch ratio",
		Name:        StatNameDispatchRatio,
		Unit:        "%",
	}, d.statDispatch)
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
