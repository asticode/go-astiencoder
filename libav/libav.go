package astilibav

import "github.com/asticode/goav/avcodec"

// PktHandler represents an object that can handle a pkt
type PktHandler interface {
	HandlePkt(pkt *avcodec.Packet)
}

// PktWriter represents an object that can write a pkt to a specific path
type PktWriter interface {
	WritePkt(pkt *avcodec.Packet, path string)
}
