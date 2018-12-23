package astilibav

import (
	"sync"

	"github.com/asticode/go-astiencoder"
)

// FiltererSwitcher represents an object that can take care of synchronizing things when switching a filterer
type FiltererSwitcher interface {
	IncIn(n astiencoder.Node)
	IncOut()
	Reset()
	ShouldIn(n astiencoder.Node) (ko bool)
	ShouldOut() (ko bool)
	Switch(cs FiltererSwitcherCallbacks)
}

// FiltererSwitcherCallbacks represents filterer switcher callbacks
type FiltererSwitcherCallbacks struct {
	In  map[astiencoder.Node]func()
	Out func()
}

type filtererSwitcher struct {
	cs FiltererSwitcherCallbacks
	is map[astiencoder.Node]int
	l  int
	m  *sync.Mutex
	o  int
	os *sync.Once
}

func newFiltererSwitcher() *filtererSwitcher {
	return &filtererSwitcher{
		is: make(map[astiencoder.Node]int),
		m:  &sync.Mutex{},
		os: &sync.Once{},
	}
}

func (s *filtererSwitcher) IncIn(n astiencoder.Node) {
	// Lock
	s.m.Lock()
	defer s.m.Unlock()

	// Node doesn't exist
	if _, ok := s.is[n]; !ok {
		s.is[n] = 0
	}

	// Increment
	s.is[n]++

	// Execute callback
	if s.l > 0 && s.is[n] == s.l {
		if fn, ok := s.cs.In[n]; ok && fn != nil {
			fn()
		}
	}
	return
}

func (s *filtererSwitcher) IncOut() {
	// Lock
	s.m.Lock()
	defer s.m.Unlock()

	// Increment
	s.o++

	// Execute callback
	if s.l > 0 && s.o == s.l && s.cs.Out != nil {
		s.cs.Out()
	}
}

func (s *filtererSwitcher) Reset() {
	// Lock
	s.m.Lock()
	defer s.m.Unlock()

	// Reset
	s.is = make(map[astiencoder.Node]int)
	s.o = 0
	s.os = &sync.Once{}
}

func (s *filtererSwitcher) ShouldIn(n astiencoder.Node) (ko bool) {
	// Lock
	s.m.Lock()
	defer s.m.Unlock()

	// Node doesn't exist
	if _, ok := s.is[n]; !ok {
		return
	}

	// Check limit
	if s.l > 0 && s.is[n]+1 > s.l {
		return true
	}
	return
}

func (s *filtererSwitcher) ShouldOut() (ko bool) {
	// Lock
	s.m.Lock()
	defer s.m.Unlock()

	// Check limit
	if s.l > 0 && s.o+1 > s.l {
		return true
	}
	return
}

func (s *filtererSwitcher) Switch(cs FiltererSwitcherCallbacks) {
	s.os.Do(func() {
		// Lock
		s.m.Lock()
		defer s.m.Unlock()

		// Set callbacks
		s.cs = cs

		// Set limit
		for _, v := range s.is {
			if v > s.l {
				s.l = v
			}
		}

		// Execute callbacks for inputs that have already reached the limit
		for n, fn := range cs.In {
			if v, ok := s.is[n]; ok && v == s.l && fn != nil {
				fn()
			}
		}

		// Execute out callback if limit has already been reached
		if s.o == s.l && cs.Out != nil {
			cs.Out()
		}
	})
}
