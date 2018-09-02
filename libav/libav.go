package astilibav

import (
	"github.com/selfmodify/goav/avcodec"
	"github.com/selfmodify/goav/avformat"
)

func init() {
	avformat.AvRegisterAll()
	avcodec.AvcodecRegisterAll()
}
