package astilibav

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

type pktTest struct {
	inputDts  int64
	inputPts  int64
	outputDts int64
	outputPts int64
	streamIdx int
}

func TestPktRestamperStartFromZero(t *testing.T) {
	pkt := astiav.AllocPacket()
	require.NotNil(t, pkt)
	defer pkt.Free()
	r := NewPktRestamperStartFromZero()
	for _, ft := range []pktTest{
		{inputDts: astiav.NoPtsValue, inputPts: astiav.NoPtsValue, outputDts: 0, outputPts: 0, streamIdx: 1},
		{inputDts: 15, inputPts: 15, outputDts: 0, outputPts: 0, streamIdx: 2},
		{inputDts: 10, inputPts: 13, outputDts: 10, outputPts: 13, streamIdx: 1},
		{inputDts: 115, inputPts: 115, outputDts: 100, outputPts: 100, streamIdx: 2},
		{inputDts: 20, inputPts: 24, outputDts: 20, outputPts: 24, streamIdx: 1},
		{inputDts: 120, inputPts: 120, outputDts: 105, outputPts: 105, streamIdx: 2},
	} {
		pkt.SetDts(ft.inputDts)
		pkt.SetPts(ft.inputPts)
		pkt.SetStreamIndex(ft.streamIdx)
		r.Restamp(pkt)
		require.Equal(t, ft.outputDts, pkt.Dts())
		require.Equal(t, ft.outputPts, pkt.Pts())
	}
}
