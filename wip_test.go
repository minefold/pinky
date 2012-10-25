package main

import (
	"testing"
)

func TestWorkInProgress(t *testing.T) {
	wipGen := NewWipGenerator()

	work1 := <-wipGen.C // start job 1
	work2 := <-wipGen.C // start job 2

	if wipGen.Count != 2 {
		t.Error("count != 2")
	}

	work1 <- true // finish job 1
	work2 <- true // finish job 2

	if wipGen.Count != 0 {
		t.Error("count != 0")
	}
}
