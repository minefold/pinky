package main

import (
	"path/filepath"
	"time"
	"syscall"
	"bufio"
	"fmt"
	"os"
)

type Server struct {
	Id string
	Path string
}

type ServerEvent struct {
  Name string
}

func (s *Server) stdoutPath() string {
  return filepath.Join(s.Path, "pipe_stdout")
}

func (s *Server) stdinPath() string {
  return filepath.Join(s.Path, "pipe_stdin")
}


func (s *Server) Monitor(c chan ServerEvent) {
	// TODO Wait for file to exist
	time.Sleep(3 * time.Second)

	stdout, err := os.OpenFile(s.stdoutPath(),
		syscall.O_NONBLOCK | syscall.O_RDONLY, 0x0)

	if err != nil {
		// TODO Handle better
		panic(err)
	}

	defer stdout.Close()

	r := bufio.NewReader(stdout)
		
	line, isPrefix, err := r.ReadLine()
	
	go func() {
		fmt.Println("going to sleep")
		time.Sleep(5 * time.Second)
		s.Stop()
	}()
	
	for err == nil && !isPrefix {
	    s := string(line)
	    fmt.Println(s)
		// c <- line
		
	    line, isPrefix, err = r.ReadLine()
	}
}

func (s *Server) Stop() {
	fmt.Println("stopping")
	
	fmt.Println(s.stdinPath())
	stdin, err := os.OpenFile(s.stdinPath(), syscall.O_WRONLY | syscall.O_APPEND, 0x0)
	fmt.Println("1")
	
	if err != nil {
		panic(err)
	}
	fmt.Println("2")
	defer stdin.Close()
	fmt.Println("3")
	
	stdin.WriteString("stop\n")
	fmt.Println("here!")
}


