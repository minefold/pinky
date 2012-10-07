package main

import (
	"testing"
)

func TestDiskCollection(t *testing.T) {
	usages, err := collectDiskUsage()
	if err != nil {
		t.Error(err)
	}

	// for i, usage := range usages {
	// 	fmt.Println(i, usage)
	// }

	if len(usages) < 2 {
		t.Error("disk usages not returned")
	}
}
