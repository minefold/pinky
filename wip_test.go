package main

import (
	"testing"
	"time"
)

type NullLog struct {
}

func (l *NullLog) Info(data map[string]interface{}) {
}

func (l *NullLog) Error(err error, data map[string]interface{}) {
}

func (l *NullLog) Out(level string, data map[string]interface{}) {
}

func TestWorkInProgress(t *testing.T) {
	plog = &NullLog{}

	wipGen := NewWipGenerator()

	work1 := <-wipGen.C // start job 1
	work2 := <-wipGen.C // start job 2

	if wipGen.Count != 2 {
		t.Error("count != 2")
	}

	close(work1) // finish job 1
	close(work2) // finish job 2

	// TODO don't sleep
	time.Sleep(10 * time.Millisecond)

	if wipGen.Count != 0 {
		t.Error("count != 0")
	}
}
