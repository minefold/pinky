package main

import (
	"testing"
	"time"
)

func TestWorkInProgress(t *testing.T) {
	wipGen := NewWipGenerator()

	work1 := <-wipGen.C // start job 1
	work2 := <-wipGen.C // start job 2

	if wipGen.Count != 2 {
		t.Error("count != 2")
	}

	close(work1) // finish job 1
	close(work2) // finish job 2

	time.Sleep(500 * time.Millisecond)

	if wipGen.Count != 0 {
		t.Error("count != 0")
	}
}
