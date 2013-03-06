package main

import ()

type DynoCrashed struct {
	MachineState
}

func (s *DynoCrashed) Name() string { return "crashed" }

func (s *DynoCrashed) Enter(machine *StateMachine) {
}
func (s *DynoCrashed) Exit() {
}
