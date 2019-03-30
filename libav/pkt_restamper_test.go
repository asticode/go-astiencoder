package astilibav

import (
	"testing"

	"github.com/asticode/goav/avcodec"
	"github.com/stretchr/testify/assert"
)

type pktTest struct {
	duration int64
	inputDts  int64
	inputPts  int64
	outputDts int64
	outputPts int64
	streamIdx int
}

func TestPktRestamperStartFromZero(t *testing.T) {
	pkt := avcodec.Packet{}
	r := NewPktRestamperStartFromZero()
	for _, ft := range []pktTest{
		{inputDts: 10, inputPts: 12, outputDts: 0, outputPts: 2, streamIdx: 1},
		{inputDts: 15, inputPts: 15, outputDts: 0, outputPts: 0, streamIdx: 2},
		{inputDts: 20, inputPts: 23, outputDts: 10, outputPts: 13, streamIdx: 1},
		{inputDts: 115, inputPts: 115, outputDts: 100, outputPts: 100, streamIdx: 2},
		{inputDts: 30, inputPts: 34, outputDts: 20, outputPts: 24, streamIdx: 1},
		{inputDts: 120, inputPts: 120, outputDts: 105, outputPts: 105, streamIdx: 2},
	} {
		pkt.SetDts(ft.inputDts)
		pkt.SetPts(ft.inputPts)
		pkt.SetStreamIndex(ft.streamIdx)
		r.Restamp(&pkt)
		assert.Equal(t, ft.outputDts, pkt.Dts())
		assert.Equal(t, ft.outputPts, pkt.Pts())
	}
}

func TestPktRestamperWithPktDuration(t *testing.T) {
	pkt := avcodec.Packet{}
	r := NewPktRestamperWithPktDuration()
	for _, ft := range []pktTest{
		{duration: 5, inputDts: 10, inputPts: 12, outputDts: 0, outputPts: 2, streamIdx: 1},
		{duration: 10, inputDts: 15, inputPts: 15, outputDts: 0, outputPts: 0, streamIdx: 2},
		{duration: 6, inputDts: 20, inputPts: 23, outputDts: 5, outputPts: 8, streamIdx: 1},
		{duration: 10, inputDts: 115, inputPts: 115, outputDts: 10, outputPts: 10, streamIdx: 2},
		{duration: 7, inputDts: 30, inputPts: 34, outputDts: 11, outputPts: 15, streamIdx: 1},
		{duration: 10, inputDts: 120, inputPts: 120, outputDts: 20, outputPts: 20, streamIdx: 2},
	} {
		pkt.SetDts(ft.inputDts)
		pkt.SetDuration(ft.duration)
		pkt.SetPts(ft.inputPts)
		pkt.SetStreamIndex(ft.streamIdx)
		r.Restamp(&pkt)
		assert.Equal(t, ft.outputDts, pkt.Dts())
		assert.Equal(t, ft.outputPts, pkt.Pts())
	}
}
