package main

import (
	"testing"
)

// idle
type TestIdleState struct {
	MachineState
	t *testing.T
}

func (s *TestIdleState) Enter(m *StateMachine) {
	s.t.Logf("idle - enter")
}
func (s *TestIdleState) Exit() {
	s.t.Logf("idle - exit")
}

// running
type TestRunningState struct {
	MachineState
	t *testing.T
	m *StateMachine
}

func (s *TestRunningState) Enter(m *StateMachine) {
	s.m = m
	s.t.Logf("running - enter")
}
func (s *TestRunningState) Exit() {
	s.t.Logf("running - exit")
}

func (s *TestRunningState) TestError() {
	err := s.m.Event("error")
	if err != nil {
		s.t.Fatal(err)
	}
}

// errored
type TestErroredState struct {
	MachineState
	t *testing.T
}

func (s *TestErroredState) Enter(m *StateMachine) {
	s.t.Logf("errored - enter")
}
func (s *TestErroredState) Exit() {
	s.t.Logf("errored - exit")
}

func TestMachineTransitions(t *testing.T) {
	// states
	idle := &TestIdleState{t: t}
	running := &TestRunningState{t: t}
	errored := &TestErroredState{t: t}

	// actions we send to the machine
	actions := []Transition{
		{Name: "start", From: idle, To: running},
	}

	// events from the machine
	events := []Transition{
		{Name: "error", To: errored},
	}

	m := StateMachine{
		Actions: actions,
		Events:  events,
	}
	m.To(idle)

	if m.State != idle {
		t.Fatalf("expected %T was %T", idle, m.State)
	}

	m.Action("start")
	if m.State != running {
		t.Fatalf("expected %T was %T", running, m.State)
	}

	running.TestError()
	if m.State != errored {
		t.Fatalf("expected %T was %T", errored, m.State)
	}
}
