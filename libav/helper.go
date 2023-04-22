package astilibav

import (
	"fmt"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
)

func durationToTimeBase(d time.Duration, t astiav.Rational) (i int64, r time.Duration) {
	// Get duration expressed in stream timebase
	// We need to make sure it's rounded to the nearest smaller int
	i = astiav.RescaleQRnd(d.Nanoseconds(), NanosecondRational, t, astiav.RoundingDown)

	// Update remainder
	r = d - time.Duration(astiav.RescaleQ(i, t, NanosecondRational))
	return
}

func emitError(target interface{}, eh *astiencoder.EventHandler, err error, format string, args ...interface{}) {
	eh.Emit(astiencoder.EventError(target, fmt.Errorf("astilibav: "+format+" failed: %w", append(args, err)...)))
}
