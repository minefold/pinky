package main

import ()

type DynoDown struct {
	MachineState
}

func (s *DynoDown) Name() string { return "down" }

func (s *DynoDown) Enter(machine *StateMachine) {
}
func (s *DynoDown) Exit() {
}
