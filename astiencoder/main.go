package main

import (
	"flag"
	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astilog"
	"github.com/pkg/errors"
)

func main() {
	// Parse flags
	flag.Parse()
	astilog.FlagInit()

	// Create configuration
	c, err := newConfiguration()
	if err != nil {
		astilog.Fatal(errors.Wrap(err, "main: creating configuration failed"))
	}

	// Create encoder
	e := astiencoder.NewEncoder(c.Encoder)
	defer e.Close()

	// Handle signals
	e.HandleSignals()

	// Start the encoder
	if err = e.Start(); err != nil {
		astilog.Fatal(errors.Wrap(err, "main: starting encoder failed"))
	}

	// Wait
	e.Wait()
}
