package main

import (
	"os/exec"
	"strconv"
	"strings"
)

type ProcUsage struct {
	Pid     int     `json:"pid"`
	PPid    int     `json:"ppid"`
	Cpu     float64 `json:"cpu"`
	Rss     int     `json:"rss"`
	User    string  `json:"user"`
	Command string  `json:"command"`
}

func CollectProcUsage() ([]ProcUsage, error) {
	out, err := exec.Command(
		"ps", "axo", "pid=,ppid=,pcpu=,rss=,user=,command=").Output()
	if err != nil {
		return nil, err
	}

	usages := make([]ProcUsage, 0, 10)

	lines := strings.Split(string(out), "\n")
	for _, l := range lines {
		// skip empty lines
		if len(l) == 0 {
			continue
		}

		// extract space separated columnds
		cols := splitColumns(l)

		// create struct
		pid, _ := strconv.Atoi(cols[0])
		ppid, _ := strconv.Atoi(cols[1])
		cpu, _ := strconv.ParseFloat(cols[2], 64)
		rss, _ := strconv.Atoi(cols[3])

		usages = append(usages, ProcUsage{
			Pid:     pid,
			PPid:    ppid,
			Cpu:     cpu,
			Rss:     rss,
			User:    cols[4],
			Command: cols[5],
		})
	}
	return usages, nil
}
