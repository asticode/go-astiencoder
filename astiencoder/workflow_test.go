package main

import (
	"testing"
)

func TestCopy(t *testing.T) {
	testJob(t, "../examples/copy.json", "testdata/copy.ts")
}

func TestMJpeg(t *testing.T) {
	testJob(t, "../examples/mjpeg.json", "testdata/copy.ts")
}
