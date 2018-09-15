package main

import (
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avformat"
	"github.com/asticode/goav/avutil"
)

type transmuxer struct {
	*astilibav.Muxer
}

func newTransmuxer(m *astilibav.Muxer) *transmuxer {
	return &transmuxer{Muxer: m}
}

// HandlePkt implements the astilibav.DemuxHandler interface
func (r *transmuxer) HandlePkt(pkt *avcodec.Packet, inStream, outStream *avformat.Stream) {
	// Modify packet
	pkt.SetStreamIndex(outStream.Index())
	pkt.SetDuration(avutil.AvRescaleQ(pkt.Duration(), inStream.TimeBase(), outStream.TimeBase()))
	pkt.SetDts(avutil.AvRescaleQ(pkt.Dts(), inStream.TimeBase(), outStream.TimeBase()))
	pkt.SetPts(avutil.AvRescaleQ(pkt.Pts(), inStream.TimeBase(), outStream.TimeBase()))
	pkt.SetPos(-1)

	// Handle pkt with muxer
	r.Muxer.HandlePkt(pkt)
}
