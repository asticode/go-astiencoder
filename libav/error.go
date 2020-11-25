package astilibav

import (
	"fmt"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avutil"
)

// AvError represents a libav error
type AvError int

// Error implements the error interface
func (e AvError) Error() string {
	return avutil.AvStrerr(int(e))
}

// Is implements the standard error interface
func (e AvError) Is(err error) bool {
	a, ok := err.(AvError)
	if !ok {
		return false
	}
	return int(a) == int(e)
}

// NewAvError creates a new av error
func NewAvError(ret int) AvError {
	return AvError(ret)
}

func emitAvError(target interface{}, eh *astiencoder.EventHandler, ret int, format string, args ...interface{}) {
	eh.Emit(astiencoder.EventError(target, fmt.Errorf("astilibav: "+format+": %w", append(args, NewAvError(ret))...)))
}
