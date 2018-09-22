package astilibav

import (
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avutil"
)

// FrameHandler represents an object that can handle a frame
type FrameHandler interface {
	HandleFrame(f *avutil.Frame)
}

// PktHandler represents an object that can handle a pkt
type PktHandler interface {
	HandlePkt(pkt *avcodec.Packet)
}
