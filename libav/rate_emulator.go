package astilibav

import (
	"context"
	"sync"
	"time"
)

type rateEmulatorAtFunc func(i interface{}) time.Time

type rateEmulatorBeforeFunc func(a, b interface{}) bool

type rateEmulatorExecFunc func(i interface{})

type rateEmulator struct {
	buffer      []interface{}
	cancel      context.CancelFunc
	ctx         context.Context
	funcAt      rateEmulatorAtFunc
	funcBefore  rateEmulatorBeforeFunc
	funcExec    rateEmulatorExecFunc
	items       []interface{}
	m           *sync.Mutex
	nextAt      time.Time
	reloadChan  chan bool
	shouldFlush bool
}

func newRateEmulator(funcAt rateEmulatorAtFunc, funcBefore rateEmulatorBeforeFunc, funcExec rateEmulatorExecFunc) *rateEmulator {
	return &rateEmulator{
		funcAt:     funcAt,
		funcBefore: funcBefore,
		funcExec:   funcExec,
		m:          &sync.Mutex{},
	}
}

func (r *rateEmulator) start(parentCtx context.Context) {
	// Create context
	r.m.Lock()
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()
	r.ctx = ctx
	r.cancel = cancel

	// Reset flush
	r.shouldFlush = false

	// Process buffer
	for _, i := range r.buffer {
		r.addUnlocked(i)
	}
	r.buffer = []interface{}{}
	r.m.Unlock()

	// Loop
	for {
		if stop := r.startLoopCycle(ctx); stop {
			break
		}
	}

	// Reset context
	r.m.Lock()
	r.ctx = nil
	r.cancel = nil
	shouldFlush := r.shouldFlush
	r.m.Unlock()

	// Flush
	if shouldFlush {
		r.flush()
	}
}

func (r *rateEmulator) startLoopCycle(ctx context.Context) (stop bool) {
	// Get next at
	r.m.Lock()
	nextAt := r.nextAt

	// Create reload chan
	r.reloadChan = make(chan bool)
	r.m.Unlock()

	// Make sure to close reload chan
	defer func() {
		// Lock
		r.m.Lock()
		defer r.m.Unlock()

		// Close
		if r.reloadChan != nil {
			close(r.reloadChan)
			r.reloadChan = nil
		}
	}()

	// No next at
	if nextAt.IsZero() {
		// Select
		select {
		case <-ctx.Done():
			stop = true
			return
		case <-r.reloadChan:
			return
		}
	}

	// Get duration
	d := time.Until(nextAt)

	// Run immediatly
	if d <= 0 {
		r.run()
		return
	}

	// Create timer
	t := time.NewTimer(d)
	defer t.Stop()

	// Select
	select {
	case <-ctx.Done():
		stop = true
		return
	case <-r.reloadChan:
		return
	case <-t.C:
		r.run()
	}
	return
}

func (r *rateEmulator) flush() {
	for {
		if stop := r.flushLoopCycle(); stop {
			break
		}
	}
}

func (r *rateEmulator) flushLoopCycle() (stop bool) {
	// Get next at
	r.m.Lock()
	nextAt := r.nextAt
	r.m.Unlock()

	// No next at
	if nextAt.IsZero() {
		stop = true
		return
	}

	// Sleep
	time.Sleep(time.Until(nextAt))

	// Run
	r.run()
	return
}

func (r *rateEmulator) run() {
	// Extract first item
	r.m.Lock()
	if len(r.items) == 0 {
		r.m.Unlock()
		return
	}
	i := r.items[0]
	if len(r.items) > 1 {
		r.items = r.items[1:]
		r.nextAt = r.funcAt(r.items[0])
	} else {
		r.items = []interface{}{}
		r.nextAt = time.Time{}
	}
	r.m.Unlock()

	// Exec
	r.funcExec(i)
}

func (r *rateEmulator) stop(shouldFlush bool) {
	// Lock
	r.m.Lock()
	defer r.m.Unlock()

	// Update flush
	r.shouldFlush = shouldFlush

	// Cancel
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

func (r *rateEmulator) add(i interface{}) {
	// Lock
	r.m.Lock()
	defer r.m.Unlock()

	// Add
	r.addUnlocked(i)
}

func (r *rateEmulator) addUnlocked(i interface{}) {
	// Item should be buffered
	if r.ctx == nil || r.ctx.Err() != nil {
		r.buffer = append(r.buffer, i)
		return
	}

	// Insert
	var inserted bool
	for idx := range r.items {
		if r.funcBefore(i, r.items[idx]) {
			r.items = append(r.items[:idx], append([]interface{}{i}, r.items[idx:]...)...)
			inserted = true
			break
		}
	}

	// No insert was made, we need to append
	if !inserted {
		r.items = append(r.items, i)
	}

	// Get next at
	nextAt := r.funcAt(r.items[0])

	// Next at hasn't change
	if r.nextAt.Equal(nextAt) {
		return
	}

	// Update next at
	r.nextAt = nextAt

	// Reload
	if r.reloadChan != nil {
		close(r.reloadChan)
		r.reloadChan = nil
	}
}
