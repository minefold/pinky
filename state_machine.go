package main

import (
	"errors"
)

type MachineState interface {
	Enter(machine *StateMachine)
	Exit()
	Name() string
}

type StateMachine struct {
	State        MachineState
	Actions      Transitions
	Events       Transitions
	OnTransition func(MachineState, MachineState)
}

type Transition struct {
	Name string
	From MachineState
	To   MachineState
}
type Transitions []Transition

func (t *Transitions) Matching(name string, from MachineState) Transitions {
	transitions := make(Transitions, 0)
	for _, i := range *t {
		if i.Name == name && (i.From == from || i.From == nil) {
			transitions = append(transitions, i)
		}
	}
	return transitions
}

// Should be called by owner of state machine
func (m *StateMachine) Action(name string) error {
	return m.transitionToFirstMatching(m.Actions, name)
}

// Should be called from inside machine states
func (m *StateMachine) Event(name string) error {
	return m.transitionToFirstMatching(m.Events, name)
}

func (m *StateMachine) To(to MachineState) {
	if m.State != nil {
		m.State.Exit()
		if m.OnTransition != nil {
			m.OnTransition(m.State, to)
		}
	}
	m.State = to
	if to != nil {
		to.Enter(m)
	}
}

func (m *StateMachine) transitionToFirstMatching(t Transitions, name string) error {
	transitions := t.Matching(name, m.State)
	l := len(transitions)
	if l == 1 {
		m.To(transitions[0].To)
		return nil
	}
	if l == 0 {
		return errors.New("No transition available")
	}
	return errors.New("Multiple transitions available")
}
