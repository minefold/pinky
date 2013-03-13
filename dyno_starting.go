package main

import ()

type DynoStarting struct {
	MachineState
	Dyno *Dyno
}

func (s *DynoStarting) Name() string { return "starting" }

func (s *DynoStarting) Enter(m *StateMachine) {
	go s.Dyno.startListeners()
	s.Dyno.startDyno()
	if err := m.Event("started"); err != nil {
		panic(err)
	}
}

func (s *DynoStarting) Exit() {
}
