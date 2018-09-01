package astiencoder

import (
	"context"
	"github.com/asticode/go-astilog"
	"sync"
	"time"
)

type encoder struct {
	busy bool
	d    *dispatcher
	m    *sync.Mutex
}

func newEncoder(d *dispatcher) *encoder {
	return &encoder{
		d: d,
		m: &sync.Mutex{},
	}
}

func (e *encoder) isBusy() bool {
	e.m.Lock()
	defer e.m.Unlock()
	return e.busy
}

// TODO Make sure the encoder shuts down gracefully when context is cancelled
func (e *encoder) execJob(ctx context.Context, j Job) (err error) {
	// Set busy state
	astilog.Debugf("astiencoder: executing job %+v", j)
	e.m.Lock()
	e.busy = true
	e.m.Unlock()
	defer func() {
		e.m.Lock()
		e.busy = false
		e.m.Unlock()
	}()

	//astitime.Sleep(ctx, 10*time.Second)
	time.Sleep(5*time.Second)
	return
}
