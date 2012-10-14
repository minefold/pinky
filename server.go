package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type Server struct {
	Id   string
	Path string
	Pid  int
	Proc *ManagedProcess
}

type ServerEvent struct {
	Event string
	At    int64
	Msg   string
}

type ServerSettings struct {
	Id       string        `json:"id"`
	Funpack  string        `json:"funpack"`
	Port     int           `json:"port"`
	Ram      RamAllocation `json:"ram"`
	Settings interface{}   `json:"settings"`
}

func AttachServer(id string, path string, pid int) *Server {
	s := &Server{Id: id, Path: path, Pid: pid}
	s.Proc = NewManagedProcess(pid)
	return s
}

func (s *Server) stdoutPath() string {
	return filepath.Join(s.Path, "pipe_stdout")
}

func (s *Server) stdinPath() string {
	return filepath.Join(s.Path, "pipe_stdin")
}

func (s *Server) processStdout(c chan ServerEvent) {
	stdout, err := os.OpenFile(s.stdoutPath(), syscall.O_RDONLY, 0x0)

	if err != nil {
		// TODO Handle better
		panic(err)
	}

	defer stdout.Close()

	r := bufio.NewReader(stdout)
	line, isPrefix, err := r.ReadLine()

	for err == nil && !isPrefix {
		event, parseErr := s.parseEvent(line)
		if parseErr == nil {
			c <- event
		} else {
			fmt.Println("Parse error", parseErr, "line:", string(line))
		}
		line, isPrefix, err = r.ReadLine()
	}

	close(c)
}

func (s *Server) Monitor() chan ServerEvent {
	// TODO Wait for file to exist
	time.Sleep(30 * time.Second)

	// we have two channels, where we copy incoming events to outgoing events
	// but on the stop event we intercept so we can make sure the server
	// finished stopping. There is probably a better way to do this

	eventsIn := make(chan ServerEvent)
	eventsOut := make(chan ServerEvent)

	go s.processStdout(eventsIn)

	go func() {
		for event := range eventsIn {
			switch event.Event {
			case "stopping":
				go s.ensureServerStopped()
			}
			eventsOut <- event
		}

		close(eventsOut)
	}()

	return eventsOut
}

func (s *Server) ensureServerStopped() {
	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(30 * time.Second)
		timeout <- true
	}()

	wait := make(chan bool, 1)
	go func() {
		s.Proc.Wait()
		wait <- true
	}()

	select {
	case <-wait:
		fmt.Println("process exited")
	case <-timeout:
		fmt.Println("timeout waiting for process exit. killing process", s.Pid)
		syscall.Kill(s.Pid, syscall.SIGTERM)
	}
}

func (s *Server) parseEvent(line []byte) (event ServerEvent, err error) {
	err = json.Unmarshal(line, &event)
	return
}

func (s *Server) Stop() {
	stdin, err := os.OpenFile(s.stdinPath(), syscall.O_WRONLY|syscall.O_APPEND, 0x0)

	if err != nil {
		panic(err)
	}
	defer stdin.Close()

	stdin.WriteString("stop\n")
	go s.ensureServerStopped()
}

func execWithOutput(cmd *exec.Cmd) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println(err)
	}
	err = cmd.Start()
	if err != nil {
		fmt.Println(err)
	}
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	cmd.Wait()
}

func (s *Server) WriteSettingsFile(
	serverPath string,
	pidFile string,
	port int,
	serverId string,
	funpack string,
	ram RamAllocation,
	settings interface{}) {

	serverFile := filepath.Join(serverPath, "server.json")
	// settingsJson, err := settings.MarshalJSON()
	// if err != nil {
	// 	panic(err)
	// }
	server := ServerSettings{
		Id:       serverId,
		Funpack:  funpack,
		Port:     port,
		Ram:      ram,
		Settings: settings,
	}
	serverJson, err := json.Marshal(server)
	if err != nil {
		panic(err)
	}

	ioutil.WriteFile(serverFile, serverJson, 0644)
}

func (s *Server) StartServerProcess(
	serverPath string,
	pidFile string,
	serverId string) (pid int) {

	serverFile := filepath.Join(serverPath, "server.json")
	command := filepath.Join(serverPath, "funpack", "bin", "run")

	// TODO some check to know if a funpack should run out of
	// a working directory like Minecraft does or out of the
	// funpack/server directory like TF2 does
	// maybe a check for the existence of a backup script
	workingDirectory := filepath.Join(serverPath, "working")

	bufferCmd, _ := filepath.Abs("bin/buffer-process")

	fmt.Println("starting", workingDirectory, bufferCmd, "-d", serverPath, "-p", pidFile, command, serverFile)
	cmd := exec.Command(bufferCmd, "-d", serverPath, "-p", pidFile, command, serverFile)
	cmd.Dir = workingDirectory

	go execWithOutput(cmd)

	// TODO wait for pid
	time.Sleep(3 * time.Second)
	_, s.Pid = readPidFromFile(pidFile)

	s.Proc = NewManagedProcess(cmd.Process.Pid)
	return s.Pid
}

func (s *Server) PrepareServerPath(serverPath string) {
	// TODO some check to know if a funpack should run out of
	// a working directory like Minecraft does or out of the
	// funpack/server directory like TF2 does
	// maybe a check for the existence of a backup script

	workingDirectory := filepath.Join(serverPath, "working")
	exec.Command("rm", "-rf", workingDirectory).Run()
	exec.Command("mkdir", "-p", workingDirectory).Run()
}

func (s *Server) DownloadFunpack(id string, dest string) {
	funpackPath := filepath.Join(dest, "funpack")

	fmt.Println("downloading funpack", id, funpackPath)

	// TODO this should be downloading/untarring from s3
	cmd := exec.Command(
		"rsync",
		"-a",
		"/home/vagrant/funpacks/minecraft-essentials/",
		funpackPath)
	execWithOutput(cmd)

	fmt.Println("downloaded funpack", id, dest)
}

func (s *Server) DownloadWorld(id string, dest string) {
	fmt.Println("downloading world", id, dest)
	fmt.Println("downloaded world", id, dest)
}

func (s *Server) BackupWorld(id string, dest string) {
	fmt.Println("backing up world", id, dest)
	fmt.Println("backed up world", id, dest)
}
