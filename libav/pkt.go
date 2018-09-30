package astilibav

import (
	"sync"

	"github.com/asticode/go-astiencoder"

	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/goav/avcodec"
)

// PktHandler represents an object that can handle a pkt
type PktHandler interface {
	HandlePkt(pkt *avcodec.Packet)
}

type pktDispatcher struct {
	c        *astiencoder.Closer
	hs       []pktDispatcherHandler
	m        *sync.Mutex
	pkt      *avcodec.Packet
	waitStat *astistat.WaitStat
}

type pktDispatcherHandler struct {
	h   PktHandler
	pkt *avcodec.Packet
	use func(pkt *avcodec.Packet) bool
}

func newPktDispatcher(c *astiencoder.Closer) (d *pktDispatcher) {
	// Create dispatcher
	d = &pktDispatcher{
		c:        c,
		m:        &sync.Mutex{},
		waitStat: astistat.NewWaitStat(),
	}

	// Create pkt
	d.pkt = avcodec.AvPacketAlloc()
	c.Add(func() error {
		avcodec.AvPacketFree(d.pkt)
		return nil
	})
	return
}

func (d *pktDispatcher) addHandler(h PktHandler, use func(pkt *avcodec.Packet) bool) {
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
		use: use,
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
		if h.use == nil || h.use(d.pkt) {
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
	d.waitStat.Add(d.pkt)
	p.Wait()
	d.waitStat.Done(d.pkt)
}

func (d *pktDispatcher) addStats(s *astistat.Stater) {
	// Add wait time
	s.AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent waiting for first child to finish processing dispatched packet",
		Label:       "Dispatch wait",
		Unit:        "%",
	}, d.waitStat)
}
