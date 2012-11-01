package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Server struct {
	Id       string
	Path     string
	Pid      int
	Port     int
	HasWorld bool
	Proc     *ManagedProcess
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

func AttachServer(id string, path string, pid int, port int) *Server {
	s := &Server{Id: id, Path: path, Pid: pid, Port: port}
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

	fmt.Println(s.Id, "STDOUT closed")

	close(c)
}

func fileExists(path string) bool {
	f, err := os.Open(path)
	if err == nil {
		f.Close()
		return true
	}
	return false
}

func waitForExist(path string) {
	t := time.NewTicker(100 * time.Millisecond)
	for _ = range t.C {
		if fileExists(path) {
			return
		}
	}
}

func (s *Server) Monitor() chan ServerEvent {
	waitForExist(s.stdoutPath())

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
	timeout := time.After(30 * time.Second)
	for {
		select {
		case <-timeout:
			fmt.Println(
				"timeout waiting for process exit. killing process", s.Pid)
			syscall.Kill(s.Pid, syscall.SIGTERM)
			return

		default:
			if !s.Proc.IsRunning() {
				fmt.Println(s.Id, "process exited")
				return
			}

			time.Sleep(500)
		}
	}
}

func (s *Server) parseEvent(line []byte) (event ServerEvent, err error) {
	err = json.Unmarshal(line, &event)
	return
}

func (s *Server) Stop() {
	stdin, err := os.OpenFile(
		s.stdinPath(), syscall.O_WRONLY|syscall.O_APPEND, 0x0)

	if err != nil {
		panic(err)
	}
	defer stdin.Close()

	stdin.WriteString("stop\n")
	s.ensureServerStopped()
}

func execWithOutput(cmd *exec.Cmd) (err error) {
	// fmt.Println(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return
	}
	err = cmd.Start()
	if err != nil {
		return
	}
	go io.Copy(os.Stdout, stdout)
	defer stdout.Close()

	go io.Copy(os.Stderr, stderr)
	defer stderr.Close()

	err = cmd.Wait()
	return
}

func (s *Server) WriteSettingsFile(funpack string,
	ram RamAllocation,
	settings interface{}) {

	serverFile := filepath.Join(s.Path, "server.json")
	// settingsJson, err := settings.MarshalJSON()
	// if err != nil {
	// 	panic(err)
	// }
	server := ServerSettings{
		Id:       s.Id,
		Funpack:  funpack,
		Port:     s.Port,
		Ram:      ram,
		Settings: settings,
	}
	serverJson, err := json.Marshal(server)
	if err != nil {
		panic(err)
	}

	ioutil.WriteFile(serverFile, serverJson, 0644)
}

func (s *Server) StartServerProcess(pidFile string) (pid int) {

	serverFile := filepath.Join(s.Path, "server.json")
	command := filepath.Join(s.Path, "funpack", "bin", "run")

	// TODO some check to know if a funpack should run out of
	// a working directory like Minecraft does or out of the
	// funpack/server directory like TF2 does
	// maybe a check for the existence of a backup script
	workingDirectory := filepath.Join(s.Path, "working")

	bufferCmd, _ := filepath.Abs("bin/buffer-process")

	fmt.Println("starting", workingDirectory, bufferCmd, "-d", s.Path, "-p", pidFile, command, serverFile)
	cmd := exec.Command(bufferCmd, "-d", s.Path, "-p", pidFile, command, serverFile)
	cmd.Dir = workingDirectory

	go execWithOutput(cmd)

	err := retry(50, 100*time.Millisecond, func() error {
		var err error
		_, s.Pid, err = readPidFromFile(pidFile)
		return err
	})
	if err != nil {
		panic(err)
	}

	s.Proc = NewManagedProcess(cmd.Process.Pid)
	return s.Pid
}

func (s *Server) PrepareServerPath() {
	// TODO some check to know if a funpack should run out of
	// a working directory like Minecraft does or out of the
	// funpack/server directory like TF2 does
	// maybe a check for the existence of a backup script

	dirs := []string{"funpack", "working"}
	for _, dirName := range dirs {
		dir := filepath.Join(s.Path, dirName)
		exec.Command("rm", "-rf", dir).Run()
		exec.Command("mkdir", "-p", dir).Run()
	}
}

func (s *Server) DownloadFunpack(funpackUrl string) {
	funpackPath := filepath.Join(s.Path, "funpack")
	err := restoreDir(funpackUrl, funpackPath)
	if err != nil {
		// TODO handle
		panic(err)
	}

	s.HasWorld = fileExists(
		filepath.Join(s.funpackPath(), "bin", "backup-paths"))
}

func (s *Server) DownloadWorld(world string) {
	restoreDirBin, _ := filepath.Abs("bin/restore-dir")

	cmd := exec.Command(restoreDirBin, world)
	cmd.Dir = filepath.Join(s.Path, "working")

	err := execWithOutput(cmd)
	if err != nil {
		panic(err)
	}
}

func (s *Server) BackupWorld(backupTime time.Time) (key string, err error) {
	if !s.HasWorld {
		return "", errors.New("Nothing to back up")
	}
	// TODO environment stuff
	environment := "development"
	bucket := "minefold-" + environment

	timestamp := backupTime.Unix()

	key = fmt.Sprintf("worlds/%s/%s.%d.tar.lzo", s.Id, s.Id, timestamp)

	uri := fmt.Sprintf("s3://%s/%s", bucket, key)

	archiveDirBin, _ := filepath.Abs("bin/archive-dir")
	cmd := exec.Command(archiveDirBin, uri, s.backupPaths())
	cmd.Dir = s.workingPath()

	err = execWithOutput(cmd)

	return
}

func restoreDir(source string, dest string) error {
	var cmd *exec.Cmd

	if strings.Contains(source, "s3://") {
		restoreDirBin, _ := filepath.Abs("bin/restore-dir")

		cmd = exec.Command(restoreDirBin, source)
		cmd.Dir = dest
	} else {
		cmd = exec.Command(
			"rsync",
			"-a",
			source,
			dest)
	}
	return execWithOutput(cmd)
}

func (s *Server) backupPaths() string {
	cmd := filepath.Join(s.funpackPath(), "bin", "backup-paths")
	paths, err := exec.Command(cmd, s.workingPath()).Output()
	if err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(paths))
}

func (s *Server) funpackPath() string {
	return filepath.Join(s.Path, "funpack")
}

func (s *Server) workingPath() string {
	return filepath.Join(s.Path, "working")
}
