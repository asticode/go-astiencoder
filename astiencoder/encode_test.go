package main

import (
	"testing"
)

func TestThumbnail(t *testing.T) {
	testJob(t, "../examples/thumbnail.json", "testdata/remux.ts")
}
