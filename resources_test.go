package main

import (
	// "fmt"
	"testing"
)

func TestDiskCollection(t *testing.T) {
	usages, err := CollectDiskUsage()
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

func TestMemoryCollection(t *testing.T) {
	usages, err := CollectProcUsage()
	if err != nil {
		t.Error(err)
	}

	// for i, usage := range usages {
	// 	fmt.Println(i, usage)
	// }

	if len(usages) < 2 {
		t.Error("memory usages not returned")
	}
}
