package main

import (
	"testing"

	"github.com/asticode/go-astiencoder"
)

func TestCopy(t *testing.T) {
	testJob(t, "../examples/copy.json", func(j astiencoder.Job) map[string]string {
		return map[string]string{
			"../examples/tmp/copy.mp4": "testdata/copy.mp4",
		}
	})
}

func TestMJpeg(t *testing.T) {
	testJob(t, "../examples/mjpeg.json", func(j astiencoder.Job) (o map[string]string) {
		return map[string]string{
			"../examples/tmp/default-0-0-1.jpeg": "testdata/default-0-0-1.jpeg",
			"../examples/tmp/default-0-1-2.jpeg": "testdata/default-0-1-2.jpeg",
			"../examples/tmp/default-0-2-3.jpeg": "testdata/default-0-2-3.jpeg",
			"../examples/tmp/default-0-3-4.jpeg": "testdata/default-0-3-4.jpeg",
			"../examples/tmp/default-0-4-5.jpeg": "testdata/default-0-4-5.jpeg",
		}
	})
}
