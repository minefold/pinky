package main

import (
	"io"
	"os"
	// "time"
)

type DynoUp struct {
	MachineState
	Dyno *Dyno
}

func (s *DynoUp) Name() string { return "up" }

func (s *DynoUp) Enter(machine *StateMachine) {
	go io.Copy(os.Stdout, s.Dyno.stdout)
	io.Copy(os.Stderr, s.Dyno.stderr)
}

func (s *DynoUp) Exit() {
}
