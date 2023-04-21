package astilibav

import (
	"testing"

	"github.com/asticode/go-astiav"
	"github.com/stretchr/testify/require"
)

type frameTest struct {
	f      func()
	input  int64
	output int64
}

func TestFrameRestamperWithFrameDuration(t *testing.T) {
	f := astiav.AllocFrame()
	require.NotNil(t, f)
	defer f.Free()
	r := NewFrameRestamperWithFrameDuration(10, func() int64 { return 1 })
	for _, ft := range []frameTest{
		{input: 0, output: 1},
		{input: 5, output: 11},
		{input: 10, output: 21},
	} {
		f.SetPts(ft.input)
		r.Restamp(f)
		require.Equal(t, ft.output, f.Pts())
	}
}

func TestFrameRestamperWithModulo(t *testing.T) {
	f := astiav.AllocFrame()
	require.NotNil(t, f)
	defer f.Free()
	r := NewFrameRestamperWithModulo(10)
	for _, ft := range []frameTest{
		{input: 0, output: 0},
		{input: 8, output: 10},
		{input: 16, output: 20},
		{input: 32, output: 30},
		{input: 40, output: 40},
		{input: 102, output: 100},
		{input: 108, output: 110},
		{input: 116, output: 120},
		{input: 132, output: 130},
		{input: 133, output: 140},
		{input: 134, output: 150},
		{input: 145, output: 160},
		{input: 156, output: 170},
		{input: 205, output: 200},
	} {
		f.SetPts(ft.input)
		r.Restamp(f)
		require.Equal(t, ft.output, f.Pts())
	}
}

func TestFrameRestamperWithOffset(t *testing.T) {
	f := astiav.AllocFrame()
	require.NotNil(t, f)
	defer f.Free()
	o := NewFrameRestamperOffseterWithStartFromZero()
	r := NewFrameRestamperWithOffset(o, 1, astiav.NewRational(1, 10))
	for _, ft := range []frameTest{
		{input: 2, output: 0},
		{input: 3, output: 1},
		{input: 4, output: 2},
		{f: func() { o.Paused() }, input: 10, output: 3},
		{input: 11, output: 4},
		{input: 12, output: 5},
	} {
		if ft.f != nil {
			ft.f()
		}
		f.SetPts(ft.input)
		r.Restamp(f)
		require.Equal(t, ft.output, f.Pts())
	}
}
