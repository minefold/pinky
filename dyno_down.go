package main

import (
	"fmt"
)

type DynoDown struct {
	MachineState
}

func (s *DynoDown) Enter(machine *StateMachine) {
	fmt.Println("idle - enter")
}
func (s *DynoDown) Exit() {
	fmt.Println("idle - exit")
}
