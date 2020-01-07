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

// IsOneOf checks whether the error is one of the specified raw return code such as avutil.AVERROR_* codes
func (e AvError) IsOneOf(rs ...int) bool {
	for _, r := range rs {
		if e == AvError(r) {
			return true
		}
	}
	return false
}

// NewAvError creates a new av error
func NewAvError(ret int) AvError {
	return AvError(ret)
}

func emitAvError(target interface{}, eh *astiencoder.EventHandler, ret int, format string, args ...interface{}) {
	eh.Emit(astiencoder.EventError(target, fmt.Errorf("astilibav: "+format+": %w", append(args, NewAvError(ret))...)))
}
