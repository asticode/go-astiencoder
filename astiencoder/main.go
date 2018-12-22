package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/go-astilog"
	"github.com/asticode/go-astitools/flag"
	"github.com/pkg/errors"
)

// Flags
var (
	job = flag.String("j", "", "the path to the job in JSON format")
)

func main() {
	// Parse flags
	s := astiflag.Subcommand()
	flag.Parse()

	// Version
	if s == "version" {
		fmt.Print(astilibav.Version)
		return
	}

	// Create configuration
	astilog.SetDefaultLogger()
	c, err := newConfiguration()
	if err != nil {
		astilog.Fatal(errors.Wrap(err, "main: creating configuration failed"))
	}

	// Create logger
	astilog.SetLogger(astilog.New(c.Logger))

	// Create event emitter
	ee := astiencoder.NewDefaultEventEmitter()

	// Add logger event handler
	ee.AddHandler(astiencoder.NewLoggerEventHandler())

	// Create workflow pool
	wp := astiencoder.NewWorkflowPool()

	// Create encoder
	e := newEncoder(c.Encoder, ee, wp)

	// Handle signals
	e.w.HandleSignals()

	// Serve workflow pool
	if err = wp.Serve(ee, c.Encoder.Server.PathWeb, func(h http.Handler) { e.w.Serve(c.Encoder.Server.Addr, h) }); err != nil {
		astilog.Fatal(errors.Wrap(err, "main: serving workflow pool failed"))
	}

	// Job has been provided
	if len(*job) > 0 {
		// Open file
		var f *os.File
		if f, err = os.Open(*job); err != nil {
			astilog.Fatal(errors.Wrapf(err, "main: opening %s failed", *job))
		}

		// Unmarshal
		var j Job
		if err = json.NewDecoder(f).Decode(&j); err != nil {
			astilog.Fatal(errors.Wrapf(err, "main: unmarshaling %s into %+v failed", *job, j))
		}

		// Add workflow
		var w *astiencoder.Workflow
		if w, err = addWorkflow("default", j, e); err != nil {
			astilog.Fatal(errors.Wrap(err, "main: adding default workflow failed"))
		}

		// Make sure the worker stops when the workflow is stopped
		c.Encoder.Exec.StopWhenWorkflowsAreStopped = true

		// Start workflow
		w.Start()
	}

	// Wait
	e.w.Wait()
}
