package astiencoder

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asticode/go-astikit"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

const (
	StatNamePSUtil = "astiencoder.ps.util"
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

type statPSUtil struct {
	started uint32
}

func newStatPSUtil() *statPSUtil {
	return &statPSUtil{}
}

func (s *statPSUtil) Start() {
	atomic.SwapUint32(&s.started, 1)
}

func (s *statPSUtil) Stop() {
	atomic.SwapUint32(&s.started, 0)
}

type statPSUtilValue struct {
	CPU    statPSUtilValueCPU    `json:"cpu"`
	Memory statPSUtilValueMemory `json:"memory"`
}

type statPSUtilValueCPU struct {
	Global     float64   `json:"global"`
	Individual []float64 `json:"individual"`
}

type statPSUtilValueMemory struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

func (s *statPSUtil) Value(delta time.Duration) interface{} {
	// Check started
	if atomic.LoadUint32(&s.started) == 0 {
		return nil
	}

	// Get value
	var v statPSUtilValue
	if vs, err := cpu.Percent(0, false); err == nil && len(vs) > 0 {
		v.CPU.Global = vs[0]
	}
	if vs, err := cpu.Percent(0, true); err == nil {
		v.CPU.Individual = vs
	}
	if vv, err := mem.VirtualMemory(); err == nil {
		v.Memory = statPSUtilValueMemory{
			Total: vv.Total,
			Used:  vv.Used,
		}
	}
	return v
}
