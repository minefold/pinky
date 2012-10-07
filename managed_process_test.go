package main

import (
	// "fmt"
	"os/exec"
	"syscall"
	"testing"
)

func TestProcessState(t *testing.T) {
	cmd := exec.Command("cat")
	cmd.Start()

	proc := NewManagedProcess(cmd.Process.Pid)

	if !proc.IsRunning() {
		t.Error("expected proc to be running")
	}

	proc.Kill(syscall.SIGTERM)
	cmd.Wait()

	if proc.IsRunning() {
		t.Error("expected proc to not be running")
	}
}
