package astilibav

import (
	"github.com/asticode/goav/avutil"
	"github.com/pkg/errors"
)

func newAvErr(ret int) error {
	return errors.New(avutil.AvStrerr(ret))
}
