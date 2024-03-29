package astilibav

import (
	"fmt"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

type FrameFiller struct {
	eh            *astiencoder.EventHandler
	fallbackFrame *astiav.Frame
	fallbackNode  astiencoder.Node
	previousFrame *astiav.Frame
	previousNode  astiencoder.Node
	onPuts        []func(f *astiav.Frame, n astiencoder.Node)
	p             *framePool
	target        interface{}
}

func NewFrameFiller(c *astikit.Closer, eh *astiencoder.EventHandler, target interface{}) *FrameFiller {
	return &FrameFiller{
		eh:     eh,
		p:      newFramePool(c),
		target: target,
	}
}

type FrameFillerFallbackFrameOptions struct {
	Frame        *astiav.Frame
	FrameAdapter FrameAdapter
	Node         astiencoder.Node
}

func (ff *FrameFiller) WithFallbackFrame(o FrameFillerFallbackFrameOptions) (dst *FrameFiller, err error) {
	// Create dst
	dst = ff

	// Get frame
	f := ff.p.get()

	// Process frame
	if o.FrameAdapter != nil {
		// Adapt frame
		if err = o.FrameAdapter(f); err != nil {
			err = fmt.Errorf("astilibav: adapting frame failed: %w", err)
			return
		}
	} else if o.Frame != nil {
		// Ref frame
		if err = f.Ref(o.Frame); err != nil {
			err = fmt.Errorf("astilibav: refing frame failed: %w", err)
			return
		}
	}

	// Store frame and node
	ff.fallbackFrame = f
	ff.fallbackNode = o.Node
	return
}

func (ff *FrameFiller) WithPreviousFrame() *FrameFiller {
	// Add on put
	ff.onPuts = append(ff.onPuts, func(f *astiav.Frame, n astiencoder.Node) {
		// Store node
		ff.previousNode = n

		// Create frame
		if ff.previousFrame == nil {
			ff.previousFrame = ff.p.get()
		} else {
			ff.previousFrame.Unref()
		}

		// Copy frame
		if err := ff.previousFrame.Ref(f); err != nil {
			emitError(ff.target, ff.eh, err, "refing frame")
			ff.p.put(ff.previousFrame)
			ff.previousFrame = nil
		}
	})
	return ff
}

func (ff *FrameFiller) Get() (*astiav.Frame, astiencoder.Node) {
	if ff.previousFrame != nil {
		return ff.previousFrame, ff.previousNode
	}
	return ff.fallbackFrame, ff.fallbackNode
}

func (ff *FrameFiller) Put(f *astiav.Frame, n astiencoder.Node) {
	// Loop through on puts
	for _, onPut := range ff.onPuts {
		onPut(f, n)
	}
}
