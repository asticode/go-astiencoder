package astiencoder

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEvent(t *testing.T) {
	// Setup
	eh := NewEventHandler()
	var es []string

	// Callbacks
	eh.Add("test-1", "test-1", func(evt Event) bool {
		es = append(es, "1")
		return true
	})
	eh.Add("test-2", "test-2", func(evt Event) bool {
		es = append(es, "2")
		return false
	})
	eh.AddForTarget("test-1", func(evt Event) bool {
		es = append(es, "3")
		return false
	})
	eh.AddForEventName("test-2", func(evt Event) bool {
		es = append(es, "4")
		return false
	})
	eh.AddForAll(func(evt Event) bool {
		es = append(es, "5")
		return false
	})

	// Emit #1
	eh.Emit(Event{
		Name:   "test-1",
		Target: "test-1",
	})
	assert.Equal(t, []string{"1", "3", "5"}, es)
	es = []string(nil)

	// Emit #2
	eh.Emit(Event{
		Name:   "test-2",
		Target: "test-2",
	})
	assert.Equal(t, []string{"2", "4", "5"}, es)
}
