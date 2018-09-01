package astiencoder

import (
	"github.com/asticode/go-astitools/worker"
)

// Encoder represents an encoder
type Encoder struct {
	s *server
	*astiworker.Worker
}

// NewEncoder creates a new encoder
func NewEncoder(c Configuration) (e *Encoder) {
	e = &Encoder{
		Worker: astiworker.NewWorker(),
	}
	e.s = newServer(c.Server, e.Worker.Serve)
	return
}

// Close implements the io.Closer interface
func (e *Encoder) Close() error {
	return nil
}

// Start starts the encoder
func (e *Encoder) Start() (err error) {
	// Start the server
	e.s.start()
	return
}
