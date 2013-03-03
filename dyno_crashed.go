package main

import (
	"fmt"
)

type DynoCrashed struct {
	MachineState
}

func (s *DynoCrashed) Enter(machine *StateMachine) {
	fmt.Println("idle - enter")
}
func (s *DynoCrashed) Exit() {
	fmt.Println("idle - exit")
}
