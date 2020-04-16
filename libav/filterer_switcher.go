package astilibav

import (
	"sync"

	"github.com/asticode/go-astiencoder"
)

type EmitFunc func(name string, payload interface{})

// FiltererSwitcher represents an object that can take care of synchronizing things when switching a filterer
type FiltererSwitcher interface {
	IncIn(n astiencoder.Node)
	IncOut()
	Reset()
	SetEmitFunc(fn EmitFunc)
	ShouldIn(n astiencoder.Node) (ko bool)
	ShouldOut() (ko bool)
	Switch()
}

type filtererSwitcher struct {
	emitFunc EmitFunc
	is       map[astiencoder.Node]int
	l        int
	m        *sync.Mutex
	o        int
	os       *sync.Once
}

func newFiltererSwitcher() *filtererSwitcher {
	return &filtererSwitcher{
		is: make(map[astiencoder.Node]int),
		m:  &sync.Mutex{},
		os: &sync.Once{},
	}
}

func (s *filtererSwitcher) SetEmitFunc(fn EmitFunc) {
	s.emitFunc = fn
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

	// Send event
	if s.l > 0 && s.is[n] == s.l {
		s.emitEventIn(n)
	}
}

func (s *filtererSwitcher) IncOut() {
	// Lock
	s.m.Lock()
	defer s.m.Unlock()

	// Increment
	s.o++

	// Send event
	if s.l > 0 && s.o == s.l {
		s.emitEventOut()
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

func (s *filtererSwitcher) Switch() {
	s.os.Do(func() {
		// Lock
		s.m.Lock()
		defer s.m.Unlock()

		// Set limit
		for _, v := range s.is {
			if v > s.l {
				s.l = v
			}
		}

		// Send events for inputs that have already reached the limit
		for n, v := range s.is {
			if v == s.l {
				s.emitEventIn(n)
			}
		}

		// Send out event if limit has already been reached
		if s.o == s.l {
			s.emitEventOut()
		}
	})
}

func (s *filtererSwitcher) emitEventIn(n astiencoder.Node) {
	if s.emitFunc == nil {
		return
	}
	s.emitFunc(EventNameFiltererSwitchInDone, n)
}

func (s *filtererSwitcher) emitEventOut() {
	if s.emitFunc == nil {
		return
	}
	s.emitFunc(EventNameFiltererSwitchOutDone, nil)
}
