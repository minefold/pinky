package main

import (
	"testing"
)

func TestMapI(t *testing.T) {
	line := `{"a": "sup", "b": 2}`

	e, _ := ParseServerEvent([]byte(line))

	if e["a"].(string) != "sup" {
		t.Errorf("expected sup was %q", e["a"])
	}
	if e["b"].(float64) != 2 {
		t.Errorf("expected 2 was %q", e["b"])
	}
}
