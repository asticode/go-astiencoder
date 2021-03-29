package astilibav

import (
	"github.com/asticode/goav/avutil"
)

// Descriptor is an object that can describe a set of parameters
type Descriptor interface {
	TimeBase() avutil.Rational
}

func NewDescriptor(timeBase avutil.Rational) Descriptor {
	return descriptor{timeBase: timeBase}
}

type descriptor struct {
	timeBase avutil.Rational
}

func (d descriptor) TimeBase() avutil.Rational {
	return d.timeBase
}
