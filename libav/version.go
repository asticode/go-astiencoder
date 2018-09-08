package astilibav

import (
	"fmt"

	"github.com/asticode/goav/avcodec"
	"github.com/asticode/goav/avdevice"
	"github.com/asticode/goav/avfilter"
	"github.com/asticode/goav/avutil"
	"github.com/asticode/goav/swresample"
	"github.com/asticode/goav/swscale"
)

// Version stores the versions
var Version = Versions{
	AvCodec:  avcodec.AvcodecVersion(),
	AvDevice: avdevice.AvdeviceVersion(),
	AvFilter: avfilter.AvfilterVersion(),
	AvUtil:   avutil.AvutilVersion(),
	Resample: swresample.SwresampleLicense(),
	SWScale:  swscale.SwscaleVersion(),
}

// Versions represents the versions
type Versions struct {
	AvCodec  uint
	AvDevice uint
	AvFilter uint
	AvUtil   uint
	Resample string
	SWScale  uint
}

// String implements the Stringer interface
func (v Versions) String() string {
	return fmt.Sprintf(`avcodec: %v
avdevice: %v
avfilter: %v
avutil: %v
resample: %v
swscale: %v
`, v.AvCodec, v.AvDevice, v.AvFilter, v.AvUtil, v.Resample, v.SWScale)
}
