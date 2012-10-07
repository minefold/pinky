package main

import (
	"syscall"
	"time"
)

type ManagedProcess struct {
	Pid int
}

func NewManagedProcess(pid int) *ManagedProcess {
	return &ManagedProcess{Pid: pid}
}

func (p *ManagedProcess) IsRunning() bool {
	return syscall.Kill(p.Pid, 0) == nil
}

func (p *ManagedProcess) Kill(sig syscall.Signal) {
	syscall.Kill(p.Pid, sig)
}

func (p *ManagedProcess) Wait() {
	ticker := time.NewTicker(500 * time.Millisecond)
	for _ = range ticker.C {
		if !p.IsRunning() {
			return
		}
	}
}
