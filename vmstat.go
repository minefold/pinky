package main

import (
	"os/exec"
	"strconv"
	"strings"
)

type VMStat struct {
	FreeMb  int `json:"freeMb"`
	IdleCpu int `json:"idleCpu"`
}

// procs -----------memory---------- ---swap-- -----io---- -system-- ----cpu----
//  r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa
//  0  0     38   1839     18     62   48   44   102   108  253  597  3  5 92  0

func GetVmStat() (*VMStat, error) {
	out, err := exec.Command("vmstat", "-SM").Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	l := lines[2]
	// extract space separated columnds
	cols := splitColumns(l)

	// create struct
	freeMb, _ := strconv.Atoi(cols[3])
	idleCpu, _ := strconv.Atoi(cols[14])

	stat := &VMStat{
		FreeMb:  freeMb,
		IdleCpu: idleCpu,
	}

	return stat, nil
}
