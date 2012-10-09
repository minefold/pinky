package main

import (
	"os/exec"
	"strconv"
	"strings"
)

type DiskUsage struct {
	Name    string `json:"name"`
	MbUsed  int    `json:"used"`
	MbTotal int    `json:"total"`
}

func splitColumns(line string, max int) []string {
	cols := make([]string, 0, max)
	for _, f := range strings.Split(line, " ") {
		if len(f) > 0 {
			cols = append(cols, f)
		}
	}
	return cols
}

func CollectDiskUsage() ([]DiskUsage, error) {
	out, err := exec.Command("df", "-m").Output()
	if err != nil {
		return nil, err
	}

	usages := make([]DiskUsage, 0, 10)

	lines := strings.Split(string(out), "\n")
	for i, l := range lines {
		// skip first and empty lines of output
		if i == 0 || len(l) == 0 {
			continue
		}

		// extract space separated columnds
		cols := splitColumns(l, 10)

		// create struct
		total, _ := strconv.Atoi(cols[1])
		used, _ := strconv.Atoi(cols[2])

		usages = append(usages, DiskUsage{
			Name:    cols[0],
			MbUsed:  used,
			MbTotal: total,
		})
		// n, err := strconv.Atoi(l)
		// if err != nil { return nil, err }
		// nums = append(nums, n)
	}
	return usages, nil
}
