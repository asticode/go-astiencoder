package main

import (
	"testing"
)

func TestCopy(t *testing.T) {
	testJob(t, "../examples/copy.json", func(j Job) map[string]string {
		return map[string]string{
			"../examples/tmp/copy.mp4": "testdata/copy.mp4",
		}
	})
}

func TestMJpeg(t *testing.T) {
	testJob(t, "../examples/mjpeg.json", func(j Job) (o map[string]string) {
		return map[string]string{
			"../examples/tmp/default-0-0-1.jpeg": "testdata/default-0-0-1.jpeg",
			"../examples/tmp/default-0-1-2.jpeg": "testdata/default-0-1-2.jpeg",
			"../examples/tmp/default-0-2-3.jpeg": "testdata/default-0-2-3.jpeg",
			"../examples/tmp/default-0-3-4.jpeg": "testdata/default-0-3-4.jpeg",
			"../examples/tmp/default-0-4-5.jpeg": "testdata/default-0-4-5.jpeg",
		}
	})
}

func TestEncode(t *testing.T) {
	testJob(t, "../examples/encode.json", func(j Job) (o map[string]string) {
		return map[string]string{
			"../examples/tmp/encode.mp4": "testdata/encode.mp4",
		}
	})
}
