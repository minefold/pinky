package main

import (
	"fmt"
	"io/ioutil"
	"syscall"
	"time"
)

type ManagedProcess struct {
	Pid int
}

func NewManagedProcess(pid int) *ManagedProcess {
	return &ManagedProcess{Pid: pid}
}

func IsRunning(pid int) bool {
	_, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))

	return err == nil
}

func (p *ManagedProcess) IsRunning() bool {
	return IsRunning(p.Pid)
}

func (p *ManagedProcess) Kill(sig syscall.Signal) {
	syscall.Kill(p.Pid, sig)
}

func (p *ManagedProcess) WaitForExit(timeout time.Duration) bool {
	return waitFor(timeout, func() bool {
		return !p.IsRunning()
	})
}
