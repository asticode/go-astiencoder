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

	// Create worker
	w := astiencoder.NewWorker(c.Encoder)
	defer w.Close()

	// Handle signals
	w.HandleSignals()

	// Start the worker
	if err = w.Start(); err != nil {
		astilog.Fatal(errors.Wrap(err, "main: starting worker failed"))
	}

	// Wait
	w.Wait()
}
