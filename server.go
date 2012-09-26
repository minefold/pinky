package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type Server struct {
	Id         string
	Path       string
	BufProcess *os.Process
	Process    *os.Process
}

type ServerEvent struct {
	Event string
	At    int64
	Msg   string
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

func (s *Server) Monitor(c chan ServerEvent) {
	// TODO Wait for file to exist
	time.Sleep(30 * time.Second)

	events := make(chan ServerEvent)
	go s.processStdout(events)

	for event := range events {
		switch event.Event {
		case "stopping":
			go s.ensureServerStopped()
		}
		c <- event
	}

	close(c)
}

func (s *Server) ensureServerStopped() {
	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(10 * time.Second)
		timeout <- true
	}()

	wait := make(chan bool, 1)
	go func() {
		_, waitErr := s.BufProcess.Wait()
		if waitErr == nil {
			wait <- true
		} else {
			fmt.Println("process wait error", waitErr)
		}
	}()

	select {
	case <-wait:
		fmt.Println("process exited")
	case <-timeout:
		fmt.Println("timeout waiting for process exit. killing process")
		s.BufProcess.Kill()
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

func (s *Server) StartServerProcess(serverPath string, pidFile string) {
	command := filepath.Join(serverPath, "funpack", "bin", "run")
	workingDirectory := filepath.Join(serverPath, "funpack")

	bufferCmd, _ := filepath.Abs("bin/buffer-process")

	fmt.Println("starting", bufferCmd, "-d", serverPath, "-p", pidFile, command)
	cmd := exec.Command(bufferCmd, "-d", serverPath, "-p", pidFile, command)
	cmd.Dir = workingDirectory

	go execWithOutput(cmd)
}

func (s *Server) PrepareServerPath(serverPath string) {
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
		"/home/vagrant/funpacks/team-fortress-2.funpack/",
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
