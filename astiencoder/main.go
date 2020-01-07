package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/asticode/go-astiencoder"
	astilibav "github.com/asticode/go-astiencoder/libav"
	"github.com/asticode/go-astikit"
)

// Flags
var (
	job = flag.String("j", "", "the path to the job in JSON format")
)

func main() {
	// Parse flags
	cmd := astikit.FlagCmd()
	flag.Parse()

	// Create logger
	l := log.New(log.Writer(), log.Prefix(), log.Flags())

	// Version
	if cmd == "version" {
		fmt.Print(astilibav.Version)
		return
	}

	// Create configuration
	c, err := newConfiguration()
	if err != nil {
		l.Fatal(fmt.Errorf("main: creating configuration failed: %w", err))
	}

	// Create event handler
	eh := astiencoder.NewEventHandler()

	// Adapt event handler
	astiencoder.LoggerEventHandlerAdapter(l, eh)

	// Create workflow pool
	wp := astiencoder.NewWorkflowPool()

	// Create encoder
	e := newEncoder(c.Encoder, eh, wp, l)

	// Handle signals
	e.w.HandleSignals()

	// Serve workflow pool
	if err = wp.Serve(eh, c.Encoder.Server.PathWeb, l, func(h http.Handler) {
		astikit.ServeHTTP(e.w, astikit.ServeHTTPOptions{
			Addr:    c.Encoder.Server.Addr,
			Handler: h,
		})
	}); err != nil {
		l.Fatal(fmt.Errorf("main: serving workflow pool failed: %w", err))
	}

	// Job has been provided
	if len(*job) > 0 {
		// Open file
		var f *os.File
		if f, err = os.Open(*job); err != nil {
			l.Fatal(fmt.Errorf("main: opening %s failed: %w", *job, err))
		}

		// Unmarshal
		var j Job
		if err = json.NewDecoder(f).Decode(&j); err != nil {
			l.Fatal(fmt.Errorf("main: unmarshaling %s into %+v failed: %w", *job, j, err))
		}

		// Add workflow
		var w *astiencoder.Workflow
		if w, err = addWorkflow("default", j, e); err != nil {
			l.Fatal(fmt.Errorf("main: adding default workflow failed: %w", err))
		}

		// Make sure the worker stops when the workflow is stopped
		c.Encoder.Exec.StopWhenWorkflowsAreStopped = true

		// Start workflow
		w.Start()
	}

	// Wait
	e.w.Wait()
}
