package astilibav

import (
	"testing"

	"github.com/asticode/goav/avutil"
	"github.com/stretchr/testify/assert"
)

type frameTest struct {
	input  int64
	output int64
}

func TestFrameRestamperWithFrameDuration(t *testing.T) {
	f := avutil.Frame{}
	r := NewFrameRestamperWithFrameDuration(10)
	for _, ft := range []frameTest{
		{input: 0, output: 0},
		{input: 5, output: 10},
		{input: 10, output: 20},
	} {
		f.SetPts(ft.input)
		r.Restamp(&f)
		assert.Equal(t, ft.output, f.Pts())
	}
}

func TestFrameRestamperWithModulo(t *testing.T) {
	f := avutil.Frame{}
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
		r.Restamp(&f)
		assert.Equal(t, ft.output, f.Pts())
	}
}
