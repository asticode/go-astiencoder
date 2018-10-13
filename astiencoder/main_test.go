package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"github.com/asticode/goav/avutil"
	"io/ioutil"
	"os"
	"testing"

	"github.com/asticode/go-astiencoder"
	"github.com/pkg/errors"
)

func init() {
	avutil.AvLogSetLevel(avutil.AV_LOG_ERROR)
}

func testJob(t *testing.T, jobPath string, assertPaths func(j Job) map[string]string) {
	// Create encoder
	cfg := &astiencoder.Configuration{}
	cfg.Exec.StopWhenWorkflowsAreStopped = true
	e := astiencoder.NewEncoder(cfg)
	defer e.Close()

	// Open job
	j, err := openJob(jobPath)
	if err != nil {
		t.Error(err)
		return
	}

	// Add workflow
	w, err := addWorkflow("test", j, e)
	if err != nil {
		t.Error(err)
		return
	}

	// Start workflow
	w.Start(e.Context())

	// Wait
	e.Wait()

	// Check expected paths
	for expectedPath, actualPath := range assertPaths(j) {
		assertFilesEqual(expectedPath, actualPath, t)
	}
}

func openJob(path string) (j Job, err error) {
	// Open
	var f *os.File
	if f, err = os.Open(path); err != nil {
		err = errors.Wrapf(err, "opening %s failed", path)
		return
	}
	defer f.Close()

	// Unmarshal job
	if err = json.NewDecoder(f).Decode(&j); err != nil {
		err = errors.Wrapf(err, "unmarshaling %s failed", path)
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
		err = errors.Wrapf(err, "reading file %s failed", path)
		return
	}

	// Hash
	h := sha1.New()
	if _, err = h.Write(b); err != nil {
		err = errors.Wrapf(err, "hashing content of file %s failed", path)
		return
	}
	hash = h.Sum(nil)
	return
}
