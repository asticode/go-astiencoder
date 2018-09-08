package astilibav

import (
	"github.com/asticode/goav/avutil"
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
