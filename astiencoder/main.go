package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/asticode/go-astiav"
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
	flag.Parse()

	// Create logger
	l := log.New(log.Writer(), log.Prefix(), log.Flags())

	// Create configuration
	c, err := newConfiguration()
	if err != nil {
		l.Fatal(fmt.Errorf("main: creating configuration failed: %w", err))
	}

	// Create event handler
	eh := astiencoder.NewEventHandler()

	// Create workflow server
	ws := astiencoder.NewServer(astiencoder.ServerOptions{Logger: l})
	ws.EventHandlerAdapter(eh)

	// Create stater
	s := astiencoder.NewStater(time.Second, eh)

	// Create encoder
	e := newEncoder(c.Encoder, eh, ws, l, s)

	// Log event handler
	defer eh.Log(l, astilibav.WithLog(astiav.LogLevelInfo)).Start(e.w.Context()).Close()

	// Handle signals
	e.w.HandleSignals()

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
		var cl *astikit.Closer
		if w, cl, err = addWorkflow("default", j, e); err != nil {
			l.Fatal(fmt.Errorf("main: adding default workflow failed: %w", err))
		}
		defer cl.Close()

		// Make sure the worker stops when the workflow is stopped
		c.Encoder.Exec.StopWhenWorkflowsAreStopped = true

		// Start stater
		go s.Start(e.w.Context())
		defer s.Stop()

		// Start workflow
		w.Start()
	}

	// Wait
	e.w.Wait()
}
