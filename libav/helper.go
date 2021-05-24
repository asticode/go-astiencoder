package astilibav

import (
	"time"

	"github.com/asticode/goav/avutil"
)

func durationToTimeBase(d time.Duration, t avutil.Rational) (i int64, r time.Duration) {
	// Get duration expressed in stream timebase
	i = avutil.AvRescaleQ(d.Nanoseconds(), nanosecondRational, t)

	// AvRescaleQ rounds to the nearest int, we need to make sure it's rounded to the nearest smaller int
	rd := time.Duration(avutil.AvRescaleQ(i, t, nanosecondRational))
	if rd > d {
		i--
	}

	// Update remainder
	r = d - time.Duration(avutil.AvRescaleQ(i, t, nanosecondRational))
	return
}
