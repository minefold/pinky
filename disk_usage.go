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

func splitColumns(line string) []string {
	cols := make([]string, 0)
	for _, f := range strings.Split(line, " ") {
		if len(f) > 0 {
			cols = append(cols, f)
		}
	}
	return cols
}

func CollectDiskUsage(path string) (*DiskUsage, error) {
	out, err := exec.Command("df", "-m", path).Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	for i, l := range lines {
		// skip first and empty lines of output
		if i == 0 || len(l) == 0 {
			continue
		}

		// extract space separated columnds
		cols := splitColumns(l)

		total, _ := strconv.Atoi(cols[1])
		used, _ := strconv.Atoi(cols[2])

		return &DiskUsage{
			Name:    cols[0],
			MbUsed:  used,
			MbTotal: total,
		}, nil
	}
	return nil, nil
}
