package astilibav

import (
	"context"

	"github.com/asticode/go-astilog"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
)

// Muxer represents a muxer
type Muxer struct {
	c         chan *avcodec.Packet
	ctxFormat *avformat.Context
	w         *astiencoder.Worker
}

// NewMuxer creates a new muxer
func NewMuxer(ctxFormat *avformat.Context, t astiencoder.CreateTaskFunc) *Muxer {
	return &Muxer{
		c:         make(chan *avcodec.Packet),
		ctxFormat: ctxFormat,
		w:         astiencoder.NewWorker(t),
	}
}

// Start starts the muxer
func (m *Muxer) Start(ctx context.Context) {
	m.w.Start(ctx, nil, func() {
		// Loop
		for {
			select {
			case pkt := <-m.c:
				// TODO Do stuff with the packet
				astilog.Warn("packet received: %p", pkt)
			case <-m.w.Ctx().Done():
				return
			}
		}
	})
}

// Stop stops the muxer
func (m *Muxer) Stop() {
	m.w.Stop()
}

// SendPkt sends a new packet to the muxer
func (m *Muxer) SendPkt(pkt *avcodec.Packet) {
	go func() {
		m.c <- pkt
	}()
}
