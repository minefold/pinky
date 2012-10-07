package main

import (
	"os/exec"
	"strconv"
	"strings"
)

type DiskUsage struct {
	Name    string
	MbUsed  int
	MbTotal int
}

func collectDiskUsage() ([]DiskUsage, error) {
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
		cols := make([]string, 0, 10)
		for _, f := range strings.Split(l, " ") {
			if len(f) > 0 {
				cols = append(cols, f)
			}
		}

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
