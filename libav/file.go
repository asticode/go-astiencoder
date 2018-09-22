package astilibav

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/go-astitools/sync"
	"github.com/asticode/go-astitools/worker"
	"github.com/asticode/goav/avcodec"
	"github.com/pkg/errors"
)

var countFileWriter uint64

// FileWriter represents an object capable of writing packets to files
type FileWriter struct {
	*astiencoder.BaseNode
	e astiencoder.EmitEventFunc
	q *astisync.CtxQueue
}

// NewFileWriter creates a new file writer
func NewFileWriter(e astiencoder.EmitEventFunc) *FileWriter {
	count := atomic.AddUint64(&countFileWriter, uint64(1))
	return &FileWriter{
		BaseNode: astiencoder.NewBaseNode(astiencoder.NodeMetadata{
			Description: "Write to files",
			Label:       fmt.Sprintf("File Writer #%d", count),
			Name:        fmt.Sprintf("file_writer_%d", count),
		}),
		e: e,
		q: astisync.NewCtxQueue(),
	}
}

// Start starts the file writer
func (w *FileWriter) Start(ctx context.Context, o astiencoder.WorkflowStartOptions, t astiencoder.CreateTaskFunc) {
	w.BaseNode.Start(ctx, o, t, func(t *astiworker.Task) {
		// Handle context
		go w.q.HandleCtx(w.Context())

		// Start queue
		w.q.Start(func(p interface{}) {
			// Assert payload
			d := p.(fileWriterData)

			// Open file
			f, err := os.Create(d.path)
			if err != nil {
				w.e(astiencoder.EventError(errors.Wrapf(err, "astilibav: opening file %s failed", d.path)))
				return
			}

			// Make sure to close the file once done
			defer f.Close()

			// TODO Write to file
			// http://www.cplusplus.com/reference/cstdio/fwrite/
		})
	})
}

type fileWriterData struct {
	path string
	pkt  *avcodec.Packet
}

// WritePkt implements the PktWriter interface
func (w *FileWriter) WritePkt(pkt *avcodec.Packet, path string) {
	w.q.Send(fileWriterData{
		path: path,
		pkt:  pkt,
	}, true)
}
