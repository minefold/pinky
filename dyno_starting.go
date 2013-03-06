package main

import (
	"bytes"
	"os"
	"os/exec"
)

type DynoStarting struct {
	MachineState
	Dyno *Dyno
}

func (s *DynoStarting) Name() string { return "starting" }

func (s *DynoStarting) Enter(m *StateMachine) {
	var mounts bytes.Buffer

	for k, v := range s.Dyno.Mounts {
		mounts.WriteString(k)
		mounts.WriteString(":")
		mounts.WriteString(v)
		mounts.WriteString(" ")
	}

	var err error

	cmd := exec.Command("bin/run-dyno", s.Dyno.Id, s.Dyno.Cmd.Path, mounts.String())
	cmd.Env = s.Dyno.Cmd.Env
	cmd.Env = append(cmd.Env, "LXC_DIR=/home/vagrant/lxc")
	cmd.Env = append(cmd.Env, "AWS_ACCESS_KEY="+os.Getenv("AWS_ACCESS_KEY"))
	cmd.Env = append(cmd.Env, "AWS_SECRET_KEY="+os.Getenv("AWS_SECRET_KEY"))

	s.Dyno.stdout, err = cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	s.Dyno.stderr, err = cmd.StderrPipe()
	if err != nil {
		panic(err)
	}

	if err := cmd.Start(); err != nil {
		panic(err)
	}

	if err := m.Event("started"); err != nil {
		panic(err)
	}
}

func (s *DynoStarting) Exit() {
}
