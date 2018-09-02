package astiencoder

import (
	"fmt"

	"github.com/selfmodify/goav/avcodec"
	"github.com/selfmodify/goav/avdevice"
	"github.com/selfmodify/goav/avfilter"
	"github.com/selfmodify/goav/avutil"
	"github.com/selfmodify/goav/swresample"
	"github.com/selfmodify/goav/swscale"
)

// Version stores the versions
var Version = Versions{
	Libs: LibsVersion{
		AvCodec:  avcodec.AvcodecVersion(),
		AvDevice: avdevice.AvdeviceVersion(),
		AvFilter: avfilter.AvfilterVersion(),
		AvUtil:   avutil.AvutilVersion(),
		Resample: swresample.SwresampleLicense(),
		SWScale:  swscale.SwscaleVersion(),
	},
}

// Versions represents the versions
type Versions struct {
	Libs LibsVersion
}

// String implements the Stringer interface
func (v Versions) String() string {
	return v.Libs.String()
}

// LibsVersion represents the libs versions
type LibsVersion struct {
	AvCodec  uint
	AvDevice uint
	AvFilter uint
	AvUtil   uint
	Resample string
	SWScale  uint
}

// String implements the Stringer interface
func (v LibsVersion) String() string {
	return fmt.Sprintf(`avcodec: %v
avdevice: %v
avfilter: %v
avutil: %v
resample: %v
swscale: %v
`, v.AvCodec, v.AvDevice, v.AvFilter, v.AvUtil, v.Resample, v.SWScale)
}
