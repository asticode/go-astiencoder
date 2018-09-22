package astiencoder

import (
	"testing"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestCloser(t *testing.T) {
	c := newCloser()
	var o []string
	c.Add(func() error {
		o = append(o, "1")
		return nil
	})
	c.Add(func() error {
		o = append(o, "2")
		return errors.New("1")
	})
	c.Add(func() error { return errors.New("2") })
	err := c.Close()
	assert.Equal(t, []string{"2", "1"}, o)
	assert.Equal(t, "2 | 1", err.Error())
}
