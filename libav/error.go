package astilibav

import (
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

// AvError represents a libav error
type AvError int

// Error implements the error interface
func (e AvError) Error() string {
	return avutil.AvStrerr(int(e))
}

func newAvError(ret int) AvError {
	return AvError(ret)
}

func emitAvError(e astiencoder.EmitEventFunc, ret int, format string, args ...interface{}) {
	e(astiencoder.EventError(errors.Wrapf(newAvError(ret), "astilibav: "+format, args...)))
}
