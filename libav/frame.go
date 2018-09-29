package astilibav

import (
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/goav/avutil"
)

// FrameHandler represents an object that can handle a frame
type FrameHandler interface {
	HandleFrame(f *avutil.Frame)
}

type frameDispatcher struct {
	c        *astiencoder.Closer
	e        astiencoder.EmitEventFunc
	f        *avutil.Frame
	hs       []frameDispatcherHandler
	m        *sync.Mutex
	waitStat *astistat.WaitStat
}

type frameDispatcherHandler struct {
	h FrameHandler
	f *avutil.Frame
}

func newFrameDispatcher(c *astiencoder.Closer, e astiencoder.EmitEventFunc) (d *frameDispatcher) {
	// Create dispatcher
	d = &frameDispatcher{
		c:        c,
		e:        e,
		m:        &sync.Mutex{},
		waitStat: astistat.NewWaitStat(),
	}

	// Create frame
	d.f = avutil.AvFrameAlloc()
	c.Add(func() error {
		avutil.AvFrameFree(d.f)
		return nil
	})
	return
}

func (d *frameDispatcher) addHandler(h FrameHandler) {
	// Lock
	d.m.Lock()
	defer d.m.Unlock()

	// Create frame
	f := avutil.AvFrameAlloc()
	d.c.Add(func() error {
		avutil.AvFrameFree(f)
		return nil
	})

	// Append
	d.hs = append(d.hs, frameDispatcherHandler{
		h: h,
		f: f,
	})
}

func (d *frameDispatcher) dispatch(r *astisync.Regulator) {
	// Lock
	d.m.Lock()

	// Make sure the frame is unref
	defer avutil.AvFrameUnref(d.f)

	// Copy handlers
	var hs []frameDispatcherHandler
	for _, h := range d.hs {
		hs = append(hs, h)
	}

	// Unlock
	d.m.Unlock()

	// Create new process
	p := r.NewProcess()

	// Add subprocesses
	p.AddSubprocesses(len(d.hs))

	// Loop through handlers
	for _, h := range hs {
		// Copy frame
		if ret := avutil.AvFrameRef(h.f, d.f); ret < 0 {
			emitAvError(d.e, ret, "avutil.AvFrameRef of %+v to %+v failed", d.f, h.f)
			p.SubprocessIsDone()
			continue
		}

		// Handle frame
		go func(h frameDispatcherHandler) {
			defer p.SubprocessIsDone()
			defer avutil.AvFrameUnref(h.f)
			h.h.HandleFrame(h.f)
		}(h)
	}

	// Wait for one of the subprocess to be done
	d.waitStat.Add(d.f)
	p.Wait()
	d.waitStat.Done(d.f)
}

func (d *frameDispatcher) addStats(s *astistat.Stater) {
	// Add wait time
	s.AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent waiting for first child to finish processing dispatched frame",
		Label:       "Dispatch wait",
		Unit:        "%",
	}, d.waitStat.StatValueFunc, d.waitStat.Reset)
}
