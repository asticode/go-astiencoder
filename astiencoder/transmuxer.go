package main

import (
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
)

type transmuxer struct {
	inStream *avformat.Stream
	nodePktHandler
	outStream *avformat.Stream
}

func newTransmuxer(h nodePktHandler, inStream, outStream *avformat.Stream) *transmuxer {
	return &transmuxer{
		inStream:       inStream,
		nodePktHandler: h,
		outStream:      outStream,
	}
}

// HandlePkt implements the astilibav.PktHandler interface
func (r *transmuxer) HandlePkt(pkt *avcodec.Packet) {
	// Modify packet
	pkt.SetStreamIndex(r.outStream.Index())
	pkt.SetDuration(avutil.AvRescaleQ(pkt.Duration(), r.inStream.TimeBase(), r.outStream.TimeBase()))
	pkt.SetDts(avutil.AvRescaleQ(pkt.Dts(), r.inStream.TimeBase(), r.outStream.TimeBase()))
	pkt.SetPts(avutil.AvRescaleQ(pkt.Pts(), r.inStream.TimeBase(), r.outStream.TimeBase()))
	pkt.SetPos(-1)

	// Handle pkt with muxer
	r.nodePktHandler.HandlePkt(pkt)
}
