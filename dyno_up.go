package main

import (
	"fmt"
)

type DynoUp struct {
	MachineState
}

func (s *DynoUp) Enter(machine *StateMachine) {
	fmt.Println("idle - enter")
}
func (s *DynoUp) Exit() {
	fmt.Println("idle - exit")
}
