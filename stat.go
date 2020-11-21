package astiencoder

import (
	"context"
	"sync"
	"time"

	"github.com/asticode/go-astikit"
)

// EventStat represents a stat event
type EventStat struct {
	Description string
	Label       string
	Name        string
	Target      interface{}
	Unit        string
	Value       interface{}
}

// Stater represents an object that can compute and handle stats
type Stater struct {
	eh *EventHandler
	m  *sync.Mutex                           // Locks ts
	ts map[*astikit.StatMetadata]interface{} // Targets indexed by stats metadata
	s  *astikit.Stater
}

// NewStater creates a new stater
func NewStater(period time.Duration, eh *EventHandler) (s *Stater) {
	s = &Stater{
		eh: eh,
		m:  &sync.Mutex{},
		ts: make(map[*astikit.StatMetadata]interface{}),
	}
	s.s = astikit.NewStater(astikit.StaterOptions{
		HandleFunc: s.handle,
		Period:     period,
	})
	return
}

// AddStats adds stats
func (s *Stater) AddStats(target interface{}, os ...astikit.StatOptions) {
	s.m.Lock()
	defer s.m.Unlock()
	for _, o := range os {
		s.ts[o.Metadata] = target
	}
	s.s.AddStats(os...)
}

// DelStats deletes stats
func (s *Stater) DelStats(target interface{}, os ...astikit.StatOptions) {
	s.m.Lock()
	defer s.m.Unlock()
	for _, o := range os {
		delete(s.ts, o.Metadata)
	}
	s.s.DelStats(os...)
}

// Start starts the stater
func (s *Stater) Start(ctx context.Context) { s.s.Start(ctx) }

// Stop stops the stater
func (s *Stater) Stop() { s.s.Stop() }

func (s *Stater) handle(stats []astikit.StatValue) {
	// No stats
	if len(stats) == 0 {
		return
	}

	// Loop through stats
	ss := []EventStat{}
	for _, stat := range stats {
		// Get target
		s.m.Lock()
		t, ok := s.ts[stat.StatMetadata]
		s.m.Unlock()

		// No target
		if !ok {
			continue
		}

		// Append
		ss = append(ss, EventStat{
			Description: stat.Description,
			Label:       stat.Label,
			Name:        stat.Name,
			Target:      t,
			Unit:        stat.Unit,
			Value:       stat.Value,
		})
	}

	// Send event
	s.eh.Emit(Event{
		Name:    EventNameStats,
		Payload: ss,
	})
}
