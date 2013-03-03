package main

// starting
type StartingState struct {
	MachineState
}

func (s *StartingState) Enter(machine *StateMachine) {
	fmt.Println("starting - enter")
}
func (s *StartingState) Exit() {
	fmt.Println("starting - exit")
}

// running
type RunningState struct {
	MachineState
}

func (s *RunningState) Enter(machine *StateMachine) {
	fmt.Println("running - enter")
}
func (s *RunningState) Exit() {
	fmt.Println("running - exit")
}

// completed
type CompletedState struct {
	MachineState
}

func (s *CompletedState) Enter(machine *StateMachine) {
	fmt.Println("completed - enter")
}
func (s *CompletedState) Exit() {
	fmt.Println("completed - exit")
}

// errored
type ErroredState struct {
	MachineState
}

func (s *ErroredState) Enter(machine *StateMachine) {
	fmt.Println("errored - enter")
}
func (s *ErroredState) Exit() {
	fmt.Println("errored - exit")
}
// states
idle := new(IdleState)
starting := new(StartingState)
listening := new(RunningState)
running := new(RunningState)
completed := new(CompletedState)
errored := new(ErroredState)

// actions we send to the machine
actions := []Transition{
	{Name: "start", From: idle, To: starting},
	{Name: "run", From: listening, To: running},
	{Name: "stop", To: completed},
}

// events from the machine
events := []Transition{
	{Name: "connected", From: starting, To: listening},
	{Name: "exit", From: running, To: completed},
	{Name: "error", To: errored},
}

tm := NewStateMachine(actions, events, idle)