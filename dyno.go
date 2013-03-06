package main

import (
	"io"
	"os/exec"
)

type Dyno struct {
	Id     string
	Cmd    *exec.Cmd
	Mounts map[string]string

	m       *StateMachine
	dynoCmd *exec.Cmd
	stdout  io.ReadCloser
	stderr  io.ReadCloser
}

func NewDyno(id string, cmd *exec.Cmd, mounts map[string]string) *Dyno {
	dyno := &Dyno{
		Id:     id,
		Cmd:    cmd,
		Mounts: mounts,
	}

	// states
	down := &DynoDown{}
	starting := &DynoStarting{Dyno: dyno}
	up := &DynoUp{Dyno: dyno}
	crashed := &DynoCrashed{}

	// actions we send to the machine
	actions := []Transition{
		{Name: "start", From: down, To: starting},
		{Name: "stop", From: up, To: down},
	}

	// events from the machine
	events := []Transition{
		{Name: "started", From: starting, To: up},
		{Name: "exit", From: up, To: down},
		{Name: "error", To: crashed},
	}

	m := &StateMachine{
		Actions: actions,
		Events:  events,
	}
	m.To(down)
	dyno.m = m
	return dyno
}

func (d *Dyno) Start() {
	if err := d.m.Action("start"); err != nil {
		panic(err)
	}
}

func (d *Dyno) Stop() {
	if err := d.m.Action("stop"); err != nil {
		panic(err)
	}
}
