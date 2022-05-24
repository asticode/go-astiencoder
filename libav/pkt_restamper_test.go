package astilibav

import (
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

type pktTest struct {
	inputDts  int64
	inputPts  int64
	outputDts int64
	outputPts int64
	streamIdx int
	t         time.Time
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

func TestFrameRestamperWithTime(t *testing.T) {
	var tm time.Time
	_now := now
	defer func() { now = _now }()
	now = func() time.Time { return tm }
	pkt := astiav.AllocPacket()
	require.NotNil(t, pkt)
	defer pkt.Free()
	r := NewPktRestamperWithTime(false, astiav.NewRational(1, 10))
	for _, v := range []pktTest{
		{t: time.Unix(0, 0), outputDts: 0},
		{t: time.Unix(0, 1e8), outputDts: 1},
		{t: time.Unix(0, 2e8), outputDts: 2},
		{t: time.Unix(0, 4e8), outputDts: 4},
		{t: time.Unix(0, 44e7), outputDts: 4},
		{t: time.Unix(0, 5e8), outputDts: 5},
		{t: time.Unix(0, 54e7), outputDts: 5},
		{t: time.Unix(0, 7e8), outputDts: 7},
	} {
		tm = v.t
		r.Restamp(pkt)
		require.Equal(t, v.outputDts, pkt.Dts())
		require.Equal(t, v.outputDts, pkt.Pts())
	}
	r = NewPktRestamperWithTime(true, astiav.NewRational(1, 10))
	for _, v := range []pktTest{
		{t: time.Unix(0, 0), outputDts: 0},
		{t: time.Unix(0, 1e8), outputDts: 1},
		{t: time.Unix(0, 2e8), outputDts: 2},
		{t: time.Unix(0, 4e8), outputDts: 3},
		{t: time.Unix(0, 44e7), outputDts: 4},
		{t: time.Unix(0, 5e8), outputDts: 5},
		{t: time.Unix(0, 54e7), outputDts: 6},
		{t: time.Unix(0, 7e8), outputDts: 7},
	} {
		tm = v.t
		r.Restamp(pkt)
		require.Equal(t, v.outputDts, pkt.Dts())
		require.Equal(t, v.outputDts, pkt.Pts())
	}
}
