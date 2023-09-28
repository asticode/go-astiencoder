package astilibav

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

var countFrameInterceptor uint64

type FrameInterceptorHandler func(f *astiav.Frame, i *FrameInterceptor) error

// FrameInterceptor represents an object capable of intercepting frames
type FrameInterceptor struct {
	*astiencoder.BaseNode
	c                   *astikit.Chan
	eh                  *astiencoder.EventHandler
	h                   FrameInterceptorHandler
	outputCtx           Context
	p                   *framePool
	statFramesProcessed uint64
	statFramesReceived  uint64
}

// FrameInterceptorOptions represents frame interceptor options
type FrameInterceptorOptions struct {
	Handler   FrameInterceptorHandler
	Node      astiencoder.NodeOptions
	OutputCtx Context
}

// NewFrameInterceptor creates a new frame interceptor
func NewFrameInterceptor(o FrameInterceptorOptions, eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) (i *FrameInterceptor) {
	// Extend node metadata
	count := atomic.AddUint64(&countFrameInterceptor, uint64(1))
	o.Node.Metadata = o.Node.Metadata.Extend(fmt.Sprintf("frame_interceptor_%d", count), fmt.Sprintf("Frame interceptor #%d", count), "Intercepts frames", "frame_interceptor")

	// Create interceptor
	i = &FrameInterceptor{
		c:         astikit.NewChan(astikit.ChanOptions{ProcessAll: true}),
		eh:        eh,
		h:         o.Handler,
		outputCtx: o.OutputCtx,
	}

	// Create base node
	i.BaseNode = astiencoder.NewBaseNode(o.Node, c, eh, s, i, astiencoder.EventTypeToNodeEventName)

	// Create frame pool
	i.p = newFramePool(i.NewChildCloser())

	// Add stat options
	i.addStatOptions()
	return
}

type FrameInterceptorStats struct {
	FramesAllocated uint64
	FramesProcessed uint64
	FramesReceived  uint64
	WorkDuration    time.Duration
}

func (i *FrameInterceptor) Stats() FrameInterceptorStats {
	return FrameInterceptorStats{
		FramesAllocated: i.p.stats().framesAllocated,
		FramesProcessed: atomic.LoadUint64(&i.statFramesProcessed),
		FramesReceived:  atomic.LoadUint64(&i.statFramesReceived),
		WorkDuration:    i.c.Stats().WorkDuration,
	}
}

func (i *FrameInterceptor) addStatOptions() {
	// Get stats
	ss := i.c.StatOptions()
	ss = append(ss, i.p.statOptions()...)
	ss = append(ss,
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames coming in per second",
				Label:       "Incoming rate",
				Name:        StatNameIncomingRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&i.statFramesReceived),
		},
		astikit.StatOptions{
			Metadata: &astikit.StatMetadata{
				Description: "Number of frames processed per second",
				Label:       "Processed rate",
				Name:        StatNameProcessedRate,
				Unit:        "fps",
			},
			Valuer: astikit.NewAtomicUint64RateStat(&i.statFramesProcessed),
		},
	)

	// Add stats
	i.BaseNode.AddStats(ss...)
}

// OutputCtx returns the output ctx
func (i *FrameInterceptor) OutputCtx() Context {
	return i.outputCtx
}

// Start starts the interceptor
func (i *FrameInterceptor) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	i.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		// Make sure to stop the chan properly
		defer i.c.Stop()

		// Start chan
		i.c.Start(i.Context())
	})
}

// HandleFrame implements the FrameHandler interface
func (i *FrameInterceptor) HandleFrame(p FrameHandlerPayload) {
	// Everything executed outside the main loop should be protected from the closer
	i.DoWhenUnclosed(func() {
		// Increment received frames
		atomic.AddUint64(&i.statFramesReceived, 1)

		// Copy frame
		f := i.p.get()
		if err := f.Ref(p.Frame); err != nil {
			emitError(i, i.eh, err, "refing frame")
			return
		}

		// Add to chan
		i.c.Add(func() {
			// Everything executed outside the main loop should be protected from the closer
			i.DoWhenUnclosed(func() {
				// Handle pause
				defer i.HandlePause()

				// Make sure to close frame
				defer i.p.put(f)

				// Increment processed frames
				atomic.AddUint64(&i.statFramesProcessed, 1)

				// Handle
				if i.h != nil {
					if err := i.h(f, i); err != nil {
						emitError(i, i.eh, err, "handling intercepted frame")
					}
				}
			})
		})
	})
}
