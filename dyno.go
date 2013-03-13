package main

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/whatupdave/dlog"
	"io"
	"net"
	"os"
	"os/exec"
	"syscall"
)

type Dyno struct {
	Id     string
	Cmd    *exec.Cmd
	Mounts map[string]string

	Stdin  chan []byte
	Stdout chan []byte

	m       *StateMachine
	dynoCmd *exec.Cmd
	process *os.Process
	stdin   io.ReadCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	conn    net.Conn

	log *dlog.Logger
}

func NewDyno(id string, cmd *exec.Cmd, mounts map[string]string) *Dyno {
	dyno := &Dyno{
		Id:     id,
		Cmd:    cmd,
		Mounts: mounts,
		Stdin:  make(chan []byte, 100),
		Stdout: make(chan []byte, 100),
		log: dlog.New(os.Stderr, map[string]interface{}{
			"dyno": id,
		}),
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
		{Name: "stopped", From: up, To: down},
		{Name: "error", To: crashed},
	}

	m := &StateMachine{
		Actions: actions,
		Events:  events,
		OnTransition: func(from MachineState, to MachineState) {
			dyno.log.Output(map[string]interface{}{
				"state-change": from.Name(),
				"to":           to.Name(),
			})
		},
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
	fmt.Println("Sending TERM to", d.process)
	d.killAll(syscall.SIGTERM)
}

func (d *Dyno) Writeln(ln string) {
	d.conn.Write([]byte(ln))
}

func (d *Dyno) startListeners() {
	cmdC := make(chan []byte, 0)
	d.startListener(fmt.Sprintf("%s/io.sock", d.socketDir()), d.Stdout)
	d.startListener(fmt.Sprintf("%s/command.sock", d.socketDir()), cmdC)

	go func() {
		for cmd := range cmdC {
			d.log.Output(map[string]interface{}{"command.sock": cmd})
			switch string(cmd) {
			case "exit":
				if err := d.m.Event("stopped"); err != nil {
					panic(err)
				}
			}
		}
	}()
}
func (d *Dyno) startListener(socket string, output chan []byte) {
	os.MkdirAll(d.socketDir(), 0777)

	l, err := net.Listen("unix", socket)
	if err != nil {
		panic(err)
	}

	d.conn, err = l.Accept()
	if err != nil {
		panic(err)
	}

	go func() {
		for line := range d.Stdin {
			d.conn.Write(line)
		}
	}()

	r := bufio.NewReader(d.conn)
	line, isPrefix, err := r.ReadLine()

	for err == nil && !isPrefix {
		output <- line
		line, isPrefix, err = r.ReadLine()
	}
}

func (d *Dyno) startDyno() {
	var err error
	var mounts bytes.Buffer

	for k, v := range d.Mounts {
		mounts.WriteString(k)
		mounts.WriteString(":")
		mounts.WriteString(v)
		mounts.WriteString(" ")
	}

	cmd := exec.Command("bin/start-container",
		d.Id, d.Cmd.Path, d.socketDir(), mounts.String())

	cmd.Env = d.Cmd.Env
	cmd.Env = append(cmd.Env, "LXC_DIR="+conf.LxcDir)
	cmd.Env = append(cmd.Env, "AWS_ACCESS_KEY="+os.Getenv("AWS_ACCESS_KEY"))
	cmd.Env = append(cmd.Env, "AWS_SECRET_KEY="+os.Getenv("AWS_SECRET_KEY"))

	d.stdout, err = cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}

	d.stderr, err = cmd.StderrPipe()
	if err != nil {
		panic(err)
	}

	cmd.Start()
}

func (d *Dyno) socketDir() string {
	return fmt.Sprintf("/tmp/%s/sockets", d.Id)
}

func (d *Dyno) killAll(sig syscall.Signal) {
	cmd := fmt.Sprintf("kill -%d -- -%d", sig, d.process.Pid)
	fmt.Println(cmd)
	exec.Command("bash", "-c", cmd).Run()
}
