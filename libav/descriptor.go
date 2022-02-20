package astilibav

import (
	"github.com/asticode/go-astiav"
)

// Descriptor is an object that can describe a set of parameters
type Descriptor interface {
	TimeBase() astiav.Rational
}

func NewDescriptor(timeBase astiav.Rational) Descriptor {
	return descriptor{timeBase: timeBase}
}

type descriptor struct {
	timeBase astiav.Rational
}

func (d descriptor) TimeBase() astiav.Rational {
	return d.timeBase
}
