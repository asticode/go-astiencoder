package astilibav

import "github.com/asticode/goav/avutil"

// Descriptor is an object that can describe a set of parameters
type Descriptor interface {
	TimeBase() avutil.Rational
}
