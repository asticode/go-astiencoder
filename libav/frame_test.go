package astilibav

import (
	"context"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astikit"
)

type frameInterceptor struct {
	*astiencoder.BaseNode
	d       *frameDispatcher
	onFrame func(p FrameHandlerPayload)
}

func newFrameInterceptor(onFrame func(p FrameHandlerPayload), eh *astiencoder.EventHandler, c *astikit.Closer, s *astiencoder.Stater) *frameInterceptor {
	// Create interceptor
	i := &frameInterceptor{onFrame: onFrame}

	// Create base node
	i.BaseNode = astiencoder.NewBaseNode(astiencoder.NodeOptions{}, c, eh, s, i, astiencoder.EventTypeToNodeEventName)

	// Create frame dispatcher
	i.d = newFrameDispatcher(i, eh)
	return i
}

func (i *frameInterceptor) Start(ctx context.Context, t astiencoder.CreateTaskFunc) {
	i.BaseNode.Start(ctx, t, func(t *astikit.Task) {
		<-ctx.Done()
	})
}

func (i *frameInterceptor) Connect(h FrameHandler) {
	// Add handler
	i.d.addHandler(h)

	// Connect nodes
	astiencoder.ConnectNodes(i, h)
}

func (i *frameInterceptor) Disconnect(h FrameHandler) {
	// Delete handler
	i.d.delHandler(h)

	// Disconnect nodes
	astiencoder.DisconnectNodes(i, h)
}

func (i *frameInterceptor) HandleFrame(p FrameHandlerPayload) {
	if i.onFrame != nil {
		i.onFrame(p)
	}
}
