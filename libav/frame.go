package astilibav

import (
	"sync"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/stat"
	"github.com/asticode/goav/avutil"
)

// FrameHandler represents an object that can handle a frame
type FrameHandler interface {
	HandleFrame(p *FrameHandlerPayload)
}

// FrameHandlerConnector represents an object that can connect with a frame handler
type FrameHandlerConnector interface {
	Connect(next FrameHandler)
}

// FrameHandlerPayload represents a FrameHandler payload
type FrameHandlerPayload struct {
	Frame *avutil.Frame
	Prev  Descriptor
}

type frameDispatcher struct {
	c            *astiencoder.Closer
	e            *astiencoder.EventEmitter
	framePool    *sync.Pool
	hs           []FrameHandler
	m            *sync.Mutex
	statDispatch *astistat.DurationRatioStat
	wg           *sync.WaitGroup
}

func newFrameDispatcher(e *astiencoder.EventEmitter, c *astiencoder.Closer) *frameDispatcher {
	return &frameDispatcher{
		c: c,
		e: e,
		framePool: &sync.Pool{New: func() interface{} {
			f := avutil.AvFrameAlloc()
			c.Add(func() error {
				avutil.AvFrameFree(f)
				return nil
			})
			return f
		}},
		m:            &sync.Mutex{},
		statDispatch: astistat.NewDurationRatioStat(),
		wg:           &sync.WaitGroup{},
	}
}

func (d *frameDispatcher) addHandler(h FrameHandler) {
	d.m.Lock()
	defer d.m.Unlock()
	d.hs = append(d.hs, h)
}

func (d *frameDispatcher) getFrame() *avutil.Frame {
	return d.framePool.Get().(*avutil.Frame)
}

func (d *frameDispatcher) putFrame(f *avutil.Frame) {
	avutil.AvFrameUnref(f)
	d.framePool.Put(f)
}

func (d *frameDispatcher) dispatch(f *avutil.Frame, prev Descriptor) {
	// Copy handlers
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

	// Wait for all previous subprocesses to be done
	// In case a brave soul tries to update this logic so that several packet can be sent to handlers in parallel, bare
	// in mind that packets must be sent in order whereas sending packets in goroutines doesn't keep this order
	d.statDispatch.Add(true)
	d.wait()
	d.statDispatch.Done(true)

	// Add subprocesses
	d.wg.Add(len(hs))

	// Loop through handlers
	for _, h := range hs {
		// Copy frame
		hF := d.getFrame()
		if ret := avutil.AvFrameRef(hF, f); ret < 0 {
			emitAvError(d.e, ret, "avutil.AvFrameRef failed")
			d.wg.Done()
			continue
		}

		// Handle frame
		go func(h FrameHandler) {
			defer d.wg.Done()
			defer d.putFrame(hF)
			h.HandleFrame(&FrameHandlerPayload{
				Frame: hF,
				Prev:  prev,
			})
		}(h)
	}
}

func (d *frameDispatcher) wait() {
	d.wg.Wait()
}

func (d *frameDispatcher) addStats(s *astistat.Stater) {
	// Add wait time
	s.AddStat(astistat.StatMetadata{
		Description: "Percentage of time spent waiting for first child to finish processing dispatched frame",
		Label:       "Dispatch ratio",
		Unit:        "%",
	}, d.statDispatch)
}
