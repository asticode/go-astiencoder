package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/asticode/go-astiencoder"
	"github.com/pkg/errors"
)

func TestRemux(t *testing.T) {
	// Create encoder
	cfg := &astiencoder.Configuration{}
	cfg.Exec.StopWhenWorkflowsAreDone = true
	e := astiencoder.NewEncoder(cfg)
	defer e.Close()

	// Set workflow builder
	e.SetWorkflowBuilder(newBuilder())

	// Open job
	j, err := openJob("../examples/remux.json")
	if err != nil {
		t.Error(err)
		return
	}

	// Create workflow
	w, err := e.NewWorkflow("test", j)
	if err != nil {
		t.Error(err)
		return
	}

	// Start workflow
	w.Start(astiencoder.StartOptions{StopChildrenWhenDone: true})

	// Wait
	e.Wait()

	// Open expected
	assertFilesEqual("testdata/remux.ts", j.Outputs["default"].URL, t)
}

func openJob(path string) (j astiencoder.Job, err error) {
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
