package astiencoder

import (
	"context"
	"sync"
	"time"

	"github.com/asticode/go-astilog"
	"github.com/pkg/errors"
	"github.com/selfmodify/goav/avcodec"
	"github.com/selfmodify/goav/avformat"
)

func init() {
	avformat.AvRegisterAll()
	avcodec.AvcodecRegisterAll()
}

// Errors
var (
	ErrExecuterIsBusy = errors.New("astiencoder: executer is busy")
)

type executer struct {
	busy bool
	ee   *eventEmitter
	m    *sync.Mutex
}

func newExecuter(ee *eventEmitter) *executer {
	return &executer{
		ee: ee,
		m:  &sync.Mutex{},
	}
}

func (e *executer) lock() error {
	e.m.Lock()
	defer e.m.Unlock()
	if e.busy {
		return ErrExecuterIsBusy
	}
	e.busy = true
	return nil
}

func (e *executer) unlock() {
	e.m.Lock()
	defer e.m.Unlock()
	e.busy = false
}

// TODO Make sure the execution is shut down gracefully when context is cancelled
func (e *executer) execJob(ctx context.Context, j Job) (err error) {
	// Open video file
	/*
		if avformat.AvformatOpenInput(&ctxtFormat, filename, nil, nil) != 0 {
			log.Println("Error: Couldn't open file.")
			return
		}
	*/

	astilog.Debugf("astiencoder: executing job %+v", j)
	//astitime.Sleep(ctx, 10*time.Second)
	time.Sleep(5 * time.Second)
	return
}
