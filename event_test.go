package astiencoder

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type mockedEventHandler struct {
	es []Event
}

func newMockedEventHandler() *mockedEventHandler {
	return &mockedEventHandler{}
}

func (h *mockedEventHandler) HandleEvent(e Event) {
	h.es = append(h.es, e)
}

func TestEvent(t *testing.T) {
	ee := NewEventEmitter()
	h := newMockedEventHandler()
	ee.AddHandler(h)
	e1 := Event{
		Name:    "1",
		Payload: "1",
	}
	e2 := EventError(errors.New("2"))
	ee.Emit(e1)
	ee.Emit(e2)
	assert.Equal(t, []Event{e1, e2}, h.es)
}
