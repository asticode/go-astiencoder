package astiencoder

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/asticode/go-astikit"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

const (
	StatNameHostUsage = "astiencoder.host.usage"
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

type StatValueHostUsage struct {
	CPU    StatValueHostUsageCPU    `json:"cpu"`
	Memory StatValueHostUsageMemory `json:"memory"`
}

type StatValueHostUsageCPU struct {
	Individual []float64 `json:"individual"`
	Process    float64   `json:"process"`
	Total      float64   `json:"total"`
}

type StatValueHostUsageMemory struct {
	Resident uint64 `json:"resident"`
	Total    uint64 `json:"total"`
	Used     uint64 `json:"used"`
	Virtual  uint64 `json:"virtual"`
}

type statPSUtil struct {
	lastTimes   *cpu.TimesStat
	lastTimesAt time.Time
	p           *process.Process
}

func newStatPSUtil() (u *statPSUtil, err error) {
	// Create util
	u = &statPSUtil{}

	// Create process
	if u.p, err = process.NewProcess(int32(os.Getpid())); err != nil {
		err = fmt.Errorf("astiencoder: creating process failed: %w", err)
		return
	}

	// Get times
	if u.lastTimes, err = u.p.Times(); err != nil {
		err = fmt.Errorf("astiencoder: getting times failed: %w", err)
		return
	}
	u.lastTimesAt = time.Now()
	return
}

func (s *statPSUtil) Value(_ time.Duration) interface{} {
	// Get process CPU
	var v StatValueHostUsage
	if t, err := s.p.Times(); err == nil {
		n := time.Now()
		v.CPU.Process = (t.Total() - t.Idle - (s.lastTimes.Total() - s.lastTimes.Idle)) / n.Sub(s.lastTimesAt).Seconds() * 100
		s.lastTimes = t
		s.lastTimesAt = n
	}

	// Get global CPU
	if ps, err := cpu.Percent(0, true); err == nil {
		v.CPU.Individual = ps
	}
	if ps, err := cpu.Percent(0, false); err == nil && len(ps) > 0 {
		v.CPU.Total = ps[0]
	}

	// Get memory
	if i, err := s.p.MemoryInfo(); err == nil {
		v.Memory.Resident = i.RSS
		v.Memory.Virtual = i.VMS
	}
	if s, err := mem.VirtualMemory(); err == nil {
		v.Memory.Total = s.Total
		v.Memory.Used = s.Used
	}
	return v
}
