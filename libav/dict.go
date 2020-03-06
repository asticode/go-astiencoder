package astilibav

import (
	"fmt"

	"github.com/asticode/goav/avutil"
)

type Dict struct {
	flags     int
	i         string
	keyValSep string
	pairsSep  string
}

func NewDict(i, keyValSep, pairsSep string, flags int) *Dict {
	return &Dict{
		flags:     flags,
		i:         i,
		keyValSep: keyValSep,
		pairsSep:  pairsSep,
	}
}

func NewDefaultDict(i string) *Dict {
	return NewDict(i, "=", ",", 0)
}

func NewDefaultDictf(format string, args ...interface{}) *Dict {
	return NewDict(fmt.Sprintf(format, args...), "=", ",", 0)
}

func (d *Dict) Parse(i **avutil.Dictionary) (err error) {
	if ret := avutil.AvDictParseString(i, d.i, d.keyValSep, d.pairsSep, d.flags); ret < 0 {
		err = fmt.Errorf("astilibav: avutil.AvDictParseString on %s failed: %w", d.i, NewAvError(ret))
		return
	}
	return
}
