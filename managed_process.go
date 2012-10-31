package main

import (
	"syscall"
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
