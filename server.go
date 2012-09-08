package main

import (
	"encoding/json"
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
  Event string
  At int64
}

func (s *Server) stdoutPath() string {
  return filepath.Join(s.Path, "pipe_stdout")
}

func (s *Server) stdinPath() string {
  return filepath.Join(s.Path, "pipe_stdin")
}


func (s *Server) Monitor(c chan ServerEvent) {
	// TODO this is testing
	go func() {
		time.Sleep(10 * time.Second)
		fmt.Println("going to sleep")
		s.Stop()
	}()
	
	// TODO Wait for file to exist
	time.Sleep(3 * time.Second)

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
			fmt.Println("Parse error", parseErr)
		}
		line, isPrefix, err = r.ReadLine()
	}
	
	fmt.Println("stdout closed")	
	
	c <- nil
}

func (s *Server) parseEvent(line []byte) (event ServerEvent, err error) {
	err = json.Unmarshal(line, &event)
	return
}

func (s *Server) Stop() {
	stdin, err := os.OpenFile(s.stdinPath(), syscall.O_WRONLY | syscall.O_APPEND, 0x0)

	if err != nil {
		panic(err)
	}
	defer stdin.Close()
	
	stdin.WriteString("stop\n")
}


