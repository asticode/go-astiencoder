package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/asticode/go-astiav"
	"github.com/asticode/go-astiencoder"
)

func init() {
	astiav.SetLogLevel(astiav.LogLevelError)
}

func testJob(t *testing.T, jobPath string, assertPaths func(j Job) map[string]string) {
	// Create logger
	l := log.New(log.Writer(), log.Prefix(), log.Flags())

	// Create event handler
	eh := astiencoder.NewEventHandler()

	// Create workflow server
	ws := astiencoder.NewServer(astiencoder.ServerOptions{Logger: l})

	// Create stater
	s := astiencoder.NewStater(2*time.Second, eh)

	// Create encoder
	cfg := &ConfigurationEncoder{}
	cfg.Exec.StopWhenWorkflowsAreStopped = true
	e := newEncoder(cfg, eh, ws, l, s)

	// Open job
	j, err := openJob(jobPath)
	if err != nil {
		t.Error(err)
		return
	}

	// Add workflow
	w, _, err := addWorkflow("test", j, e)
	if err != nil {
		t.Error(err)
		return
	}

	// Start workflow
	w.Start()

	// Wait
	e.w.Wait()

	// Check expected paths
	for expectedPath, actualPath := range assertPaths(j) {
		assertFilesEqual(expectedPath, actualPath, t)
	}
}

func openJob(path string) (j Job, err error) {
	// Open
	var f *os.File
	if f, err = os.Open(path); err != nil {
		err = fmt.Errorf("opening %s failed: %w", path, err)
		return
	}
	defer f.Close()

	// Unmarshal job
	if err = json.NewDecoder(f).Decode(&j); err != nil {
		err = fmt.Errorf("unmarshaling %s failed: %w", path, err)
		return
	}

	// Update paths
	for k, v := range j.Inputs {
		v.URL = "../" + v.URL
		j.Inputs[k] = v
	}
	for k, v := range j.Outputs {
		v.URL = "../" + v.URL
		j.Outputs[k] = v
	}
	return
}

func assertFilesEqual(expected, actual string, t *testing.T) {
	// Expected hash
	expectedHash, err := hashFileContent(expected)
	if err != nil {
		t.Error(err)
		return
	}

	// Actual hash
	actualHash, err := hashFileContent(actual)
	if err != nil {
		t.Error(err)
		return
	}

	// Compare hash
	if !bytes.Equal(expectedHash, actualHash) {
		t.Errorf("expected hash (%s) != actual hash (%s)", expectedHash, actualHash)
		return
	}
}

func hashFileContent(path string) (hash []byte, err error) {
	// Read
	var b []byte
	if b, err = ioutil.ReadFile(path); err != nil {
		err = fmt.Errorf("reading file %s failed: %w", path, err)
		return
	}

	// Hash
	h := sha1.New()
	if _, err = h.Write(b); err != nil {
		err = fmt.Errorf("hashing content of file %s failed: %w", path, err)
		return
	}
	hash = h.Sum(nil)
	return
}
