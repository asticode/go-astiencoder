package astiencoder

import (
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"time"

	"github.com/asticode/go-astikit"
)

type WorkflowRecording struct {
	c      *astikit.Chan
	cancel context.CancelFunc
	ctx    context.Context
	l      astikit.SeverityLogger
	o      WorkflowRecordingOptions
	path   string
	w      *csv.Writer
	wf     *Workflow
}

type WorkflowRecordingOptions struct {
	Dst    string
	Logger astikit.StdLogger
}

func (w *Workflow) NewRecording(o WorkflowRecordingOptions) (r *WorkflowRecording) {
	// Create recording
	r = &WorkflowRecording{
		c: astikit.NewChan(astikit.ChanOptions{
			ProcessAll: true,
		}),
		l:  astikit.AdaptStdLogger(o.Logger),
		o:  o,
		wf: w,
	}

	// Adapt event handler
	serverEventHandlerAdapter(w.eh, func(name string, payload interface{}) {
		// Recording not started
		if r.ctx == nil {
			return
		}

		// Write
		if err := r.write(name, payload); err != nil {
			r.l.Error(fmt.Errorf("astiencoder: writing to recording failed: %w", err))
		}
	})
	return
}

func (r *WorkflowRecording) Start(ctx context.Context) (err error) {
	// Recording already started
	if r.ctx != nil {
		return
	}

	// Create context
	r.ctx, r.cancel = context.WithCancel(ctx)

	// Make sure to reset context
	defer func() {
		r.cancel()
		r.ctx = nil
		r.cancel = nil
	}()

	// Create destination
	var f *os.File
	if r.o.Dst != "" {
		if f, err = os.Create(r.o.Dst); err != nil {
			err = fmt.Errorf("astiencoder: creating %s failed: %w", r.o.Dst, err)
			return
		}
	} else {
		if f, err = ioutil.TempFile("", "astiencoder-recording-*.csv"); err != nil {
			err = fmt.Errorf("astiencoder: creating temp file failed: %w", err)
			return
		}
	}
	defer f.Close()

	// Update path
	r.path = f.Name()

	// Create csv writer
	r.w = csv.NewWriter(f)

	// Write init
	if err = r.write("init", newServerWorkflow(r.wf)); err != nil {
		err = fmt.Errorf("astiencoder: adding init failed: %w", err)
		return
	}

	// Start chan
	r.c.Start(r.ctx)

	// Reset chan
	r.c.Reset()

	// Flush csv
	r.w.Flush()

	// Reset csv writer
	r.w = nil
	return
}

func (r *WorkflowRecording) write(name string, payload interface{}) (err error) {
	// Marshal payload
	var b []byte
	if b, err = json.Marshal(payload); err != nil {
		err = fmt.Errorf("astiencoder: marshaling failed: %w", err)
		return
	}

	// Write
	r.c.Add(func() {
		r.w.Write([]string{strconv.Itoa(int(time.Now().UTC().Unix())), name, base64.StdEncoding.EncodeToString(b)}) //nolint:errcheck
		r.w.Flush()
	})
	return
}

func (r *WorkflowRecording) Stop() {
	// Cancel context
	if r.cancel != nil {
		r.cancel()
	}
}
