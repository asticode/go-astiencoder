package main

import (
	"bytes"
	"sync/atomic"
	"text/template"

	"github.com/asticode/go-astiencoder"
	"github.com/asticode/goav/avcodec"
	"github.com/pkg/errors"
)

type fileWriter struct {
	nodePktWriter
	count uint64
	e     astiencoder.EmitEventFunc
	t     *template.Template
}

func newFileWriter(w nodePktWriter, url string, e astiencoder.EmitEventFunc) (s *fileWriter, err error) {
	// Create file writer
	s = &fileWriter{
		e:             e,
		nodePktWriter: w,
	}

	// Parse template
	if s.t, err = template.New("").Parse(url); err != nil {
		err = errors.Wrapf(err, "main: parsing url %s as template failed", url)
		return
	}
	return
}

// HandlePkt implements the astilibav.PktHandler interface
func (s *fileWriter) HandlePkt(pkt *avcodec.Packet) {
	// Increment count
	c := atomic.AddUint64(&s.count, 1)

	// Create data
	d := map[string]interface{}{
		"idx": c,
	}

	// Execute template
	buf := &bytes.Buffer{}
	if err := s.t.Execute(buf, d); err != nil {
		s.e(astiencoder.EventError(errors.Wrapf(err, "main: executing template on data %+v failed", d)))
		return
	}

	// Write pkt
	s.WritePkt(pkt, buf.String())
}
