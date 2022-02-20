package astilibav

import (
	"fmt"

	"github.com/asticode/go-astiav"
)

type Dictionary struct {
	content   string
	flags     astiav.DictionaryFlags
	keyValSep string
	pairsSep  string
}

func NewDictionary(content, keyValSep, pairsSep string, flags astiav.DictionaryFlags) *Dictionary {
	return &Dictionary{
		content:   content,
		flags:     flags,
		keyValSep: keyValSep,
		pairsSep:  pairsSep,
	}
}

func NewDefaultDictionary(i string) *Dictionary {
	return NewDictionary(i, "=", ",", 0)
}

func NewDefaultDictionaryf(format string, args ...interface{}) *Dictionary {
	return NewDictionary(fmt.Sprintf(format, args...), "=", ",", 0)
}

func (d *Dictionary) parse() (dd *astiav.Dictionary, err error) {
	dd = astiav.NewDictionary()
	if err = dd.ParseString(d.content, d.keyValSep, d.pairsSep, d.flags); err != nil {
		err = fmt.Errorf("astilibav: parsing dictionary content failed: %w", err)
		return
	}
	return
}
