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

func (h *mockedEventHandler) handleEvent() (isBlocking bool, fn func(e Event)) {
	return true, func(e Event) {
		h.es = append(h.es, e)
	}
}

func TestEvent(t *testing.T) {
	ee := newEventEmitter()
	h := newMockedEventHandler()
	ee.addHandler(LoggerHandleEventFunc)
	ee.addHandler(h.handleEvent)
	e1 := Event{
		Name:    "1",
		Payload: "1",
	}
	e2 := EventError(errors.New("2"))
	ee.emit(e1)
	ee.emit(e2)
	assert.Equal(t, []Event{e1, e2}, h.es)
}
