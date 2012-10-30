package main

import (
	"fmt"
	"testing"
)

func AssertInt(t *testing.T, actual int, expected int) {
	if expected != actual {
		t.Error("Expected", expected, "actual", actual)
	}
}

func TestIntPool(t *testing.T) {
	p := NewIntPool(100, 3, 40, []int{140})
	// pool should be 100, 140, 180 minus 140

	AssertInt(t, <-p, 100)
	AssertInt(t, <-p, 180)

	p <- 140 // throw num back into the pool
	AssertInt(t, <-p, 140)

	select {
	case num := <-p:
		t.Error(fmt.Sprintf("Got %d, should have blocked", num))
	default:
	}
}
