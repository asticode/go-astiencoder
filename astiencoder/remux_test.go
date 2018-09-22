package main

import (
	"testing"
)

func TestRemux(t *testing.T) {
	testJob(t, "../examples/remux.json", "testdata/remux.ts")
}
