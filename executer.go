package astiencoder

import (
	"context"
	"fmt"
	"sync"

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
	// Open input
	var ctxFormat *avformat.Context
	if err = ctxFunc(ctx, func() error {
		if avformat.AvformatOpenInput(&ctxFormat, j.URL, nil, nil) != 0 {
			return  fmt.Errorf("astiencoder: avformat.AvformatOpenInput on %s failed", j.URL)
		}
		return nil
	}); err != nil {
		return
	}
	// For now it panics
	// defer ctxFormat.AvformatCloseInput()

	// Retrieve stream information
	if err = ctxFunc(ctx, func() error {
		if ctxFormat.AvformatFindStreamInfo(nil) < 0 {
			return fmt.Errorf("astiencoder: ctxFormat.AvformatFindStreamInfo on %s failed", j.URL)
		}
		return nil
	}); err != nil {
		return
	}

	// Dump information about file onto standard error
	ctxFormat.AvDumpFormat(0, j.URL, 0)
	return
}
