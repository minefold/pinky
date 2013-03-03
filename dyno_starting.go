package main

import (
	"fmt"
)

type DynoStarting struct {
	MachineState
}

func (s *DynoStarting) Enter(machine *StateMachine) {
	fmt.Println("idle - enter")
}
func (s *DynoStarting) Exit() {
	fmt.Println("idle - exit")
}
