package astilibav

import (
	"sync"

	"github.com/asticode/go-astiencoder"

	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
)

// PktHandler represents an object that can handle a pkt
type PktHandler interface {
	HandlePkt(pkt *avcodec.Packet)
}

type pktDispatcher struct {
	c            *astiencoder.Closer
	hs           []pktDispatcherHandler
	m            *sync.Mutex
	pkt          *avcodec.Packet
	statDispatch *astistat.DurationRatioStat
}

type pktDispatcherHandler struct {
	h   PktHandler
	pkt *avcodec.Packet
}

func newPktDispatcher(c *astiencoder.Closer) (d *pktDispatcher) {
	// Create dispatcher
	d = &pktDispatcher{
		c:            c,
		m:            &sync.Mutex{},
		statDispatch: astistat.NewDurationRatioStat(),
	}

	// Create pkt
	d.pkt = avcodec.AvPacketAlloc()
	c.Add(func() error {
		avcodec.AvPacketFree(d.pkt)
		return nil
	})
	return
}

func (d *pktDispatcher) addHandler(h PktHandler) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Create pkt
	pkt := avcodec.AvPacketAlloc()
	d.c.Add(func() error {
		avcodec.AvPacketFree(pkt)
		return nil
	})

	// Append
	d.hs = append(d.hs, pktDispatcherHandler{
		h:   h,
		pkt: pkt,
	})
}

func (d *pktDispatcher) dispatch(r *astisync.Regulator) {
	// Lock
	d.m.Lock()

	// Make sure the pkt is unref
	defer d.pkt.AvPacketUnref()

	// Copy handlers
	var hs []pktDispatcherHandler
	for _, h := range d.hs {
		v, ok := h.h.(PktCond)
		if !ok || v.UsePkt(d.pkt) {
			hs = append(hs, h)
		}
	}

	// Unlock
	d.m.Unlock()

	// Create new process
	p := r.NewProcess()

	// Add subprocesses
	p.AddSubprocesses(len(hs))

	// Loop through handlers
	for _, h := range hs {
		// Copy pkt
		h.pkt.AvPacketRef(d.pkt)

		// Handle pkt
		go func(h pktDispatcherHandler) {
			defer p.SubprocessIsDone()
			defer h.pkt.AvPacketUnref()
			h.h.HandlePkt(h.pkt)
		}(h)
	}

	// Wait for one of the subprocess to be done
	d.statDispatch.Add(d.pkt)
	p.Wait()
	d.statDispatch.Done(d.pkt)
}

func (d *pktDispatcher) addStats(s *astistat.Stater) {
	// Add wait time
	s.AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent waiting for first child to finish processing dispatched packet",
		Label:       "Dispatch ratio",
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

// UsePkt implements the PktCond interface
func (c *pktCond) UsePkt(pkt *avcodec.Packet) bool {
	return pkt.StreamIndex() == c.i.Index()
}
