package main

import (
	"testing"
)

func TestRemux(t *testing.T) {
	testJob(t, "../examples/remux.json", "testdata/remux.ts")
}

func TestThumbnail(t *testing.T) {
	testJob(t, "../examples/thumbnail.json", "testdata/remux.ts")
}
