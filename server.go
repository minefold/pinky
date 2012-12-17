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
	Log      *Logger
	State    string
}

type ServerEvent struct {
	Ts    time.Time
	Event string
	Msg   string

	// these fields for the player events
	Username  string
	Usernames []string

	// these fields for the settings_changed events
	Actor string
	Key   string
	Value string
}

type ServerSettings struct {
	Id       string        `json:"id"`
	Funpack  string        `json:"funpack"`
	Port     int           `json:"port"`
	Ram      RamAllocation `json:"ram"`
	Settings interface{}   `json:"settings"`
}

type BackupInfo struct {
	Paths []string `json:"paths"`
}

func AttachServer(id string, path string, pid int, port int) *Server {
	s := &Server{Id: id, Path: path, Pid: pid, Port: port}
	s.Proc = NewManagedProcess(pid)
	s.Log = NewLog(map[string]interface{}{
		"serverId": id,
	})
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
		plog.Error(err, map[string]interface{}{
			"event":    "process_stdout",
			"serverId": s.Id,
		})
		return
	}

	defer stdout.Close()

	r := bufio.NewReader(stdout)
	line, isPrefix, err := r.ReadLine()

	for err == nil && !isPrefix {
		event, parseErr := s.parseEvent(line)
		if parseErr == nil {
			c <- event
		} else {
			plog.Info(map[string]interface{}{
				"event":    "server_log_ignored",
				"serverId": s.Id,
				"message":  string(line),
				"parseErr": parseErr,
			})
		}
		line, isPrefix, err = r.ReadLine()
	}

	plog.Info(map[string]interface{}{
		"event":    "server_stdout_exit",
		"serverId": s.Id,
	})

	close(c)
}

func fileExists(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}

func waitForExist(path string, wait time.Duration) error {
	ticker := time.NewTicker(1000 * time.Millisecond)
	timeout := time.NewTimer(wait)

	for {
		select {
		case <-timeout.C:
			return errors.New("timeout")

		case <-ticker.C:
			if fileExists(path) {
				return nil
			}
		}
	}

	return nil
}

func (s *Server) Monitor() (chan ServerEvent, error) {
	if err := waitForExist(s.stdoutPath(), 15*time.Second); err != nil {
		return nil, err
	}

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
				go s.EnsureServerStopped()
			}
			eventsOut <- event
		}

		close(eventsOut)
	}()

	return eventsOut, nil
}

func (s *Server) EnsureServerStopped() {
	time.Sleep(30 * time.Second)
	if !s.Proc.IsRunning() {
		return
	}

	time.Sleep(60 * time.Second)
	if !s.Proc.IsRunning() {
		return
	}
	plog.Info(map[string]interface{}{
		"event":  "server_exit_timeout",
		"server": s.Id,
		"pid":    s.Pid,
		"action": "TERM",
	})
	s.Kill(syscall.SIGTERM)

	time.Sleep(60 * time.Second)
	if !s.Proc.IsRunning() {
		return
	}
	for {
		plog.Info(map[string]interface{}{
			"event":  "server_exit_timeout",
			"server": s.Id,
			"pid":    s.Pid,
			"action": "KILL",
		})
		s.Kill(syscall.SIGKILL)
		if !s.Proc.IsRunning() {
			return
		}
		time.Sleep(60 * time.Second)
	}
}

func (s *Server) parseEvent(line []byte) (event ServerEvent, err error) {
	err = json.Unmarshal(line, &event)
	return
}

func (s *Server) Stop() {
	s.Writeln("stop")
	s.EnsureServerStopped()
}

func (s *Server) Broadcast(message string) {
	s.Writeln("say " + message)
}

func (s *Server) Tell(username, message string) {
	s.Writeln("tell " + username + " " + message)
}

func (s *Server) Kick(username, message string) {
	s.Writeln("kick " + username + " " + message)
}

func (s *Server) ListPlayers() {
	s.Writeln("list")
}

func (s *Server) Writeln(line string) error {
	stdin, err := os.OpenFile(
		s.stdinPath(), syscall.O_WRONLY|syscall.O_APPEND, 0x0)

	if err != nil {
		return err
	}
	defer stdin.Close()

	stdin.WriteString(line + "\n")
	return nil
}

func (s *Server) Kill(sig syscall.Signal) {
	syscall.Kill(-s.Pid, sig)
}

func execWithOutput(cmd *exec.Cmd, outWriter io.Writer, errWriter io.Writer) (err error) {
	plog.Info(map[string]interface{}{
		"event": "exec_command",
		"cmd":   fmt.Sprintf("[%s] %s %v", cmd.Dir, cmd.Path, cmd.Args),
	})

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
	go io.Copy(outWriter, stdout)
	defer stdout.Close()

	go io.Copy(errWriter, stderr)
	defer stderr.Close()

	err = cmd.Wait()
	return
}

func (s *Server) WriteSettingsFile(funpack string,
	ram RamAllocation,
	settings interface{}) error {

	serverFile := filepath.Join(s.Path, "server.json")

	server := ServerSettings{
		Id:       s.Id,
		Funpack:  funpack,
		Port:     s.Port,
		Ram:      ram,
		Settings: settings,
	}
	serverJson, err := json.Marshal(server)
	if err != nil {
		return err
	}

	ioutil.WriteFile(serverFile, serverJson, 0644)
	return nil
}

func (s *Server) StartServerProcess(pidFile string) (pid int, err error) {
	serverFile := filepath.Join(s.Path, "server.json")
	command := filepath.Join(s.Path, "funpack", "bin", "run")

	// TODO some check to know if a funpack should run out of
	// a working directory like Minecraft does or out of the
	// funpack/server directory like TF2 does
	// maybe a check for the existence of a backup script
	workingDirectory := filepath.Join(s.Path, "working")

	bufferCmd, _ := filepath.Abs("bin/buffer-process")

	plog.Info(map[string]interface{}{
		"event":      "starting_server",
		"workingDir": workingDirectory,
		"pidFile":    pidFile,
		"serverFile": serverFile,
	})

	cmd := exec.Command(bufferCmd, "-d", s.Path, "-p", pidFile, command, serverFile)
	cmd.Dir = workingDirectory

	go execWithOutput(cmd, os.Stdout, os.Stderr)

	err = retry(50, 100*time.Millisecond, func(retries int) error {
		var err error
		_, s.Pid, err = readPidFromFile(pidFile)
		return err
	})
	if err != nil {
		// TODO recover
		return 0, err
	}

	s.Proc = NewManagedProcess(cmd.Process.Pid)
	return s.Pid, nil
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

func (s *Server) DownloadFunpack(funpackUrl string) error {
	err := restoreDir(funpackUrl, s.funpackPath())
	if err != nil {
		return err
	}

	s.HasWorld = fileExists(s.funpackPath("bin", "backup"))

	plog.Info(map[string]interface{}{
		"event": "Gemfile detected",
	})

	if fileExists(s.funpackPath("Gemfile")) {
		cmd := exec.Command("bundle", "install")
		cmd.Dir = s.funpackPath()
		output, err := cmd.Output()
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":  "funpack_import_failed",
				"output": output,
			})
			return err
		}
	}
	return nil
}

func (s *Server) DownloadWorld(world string) error {
	return restoreDir(world, filepath.Join(s.Path, "working"))
}

func (s *Server) BackupWorld(backupTime time.Time) (url string, err error) {
	if !s.HasWorld {
		return "", errors.New("Nothing to back up")
	}

	bucket := os.Getenv("BACKUP_BUCKET")
	if bucket == "" {
		bucket = "minefold-development"
	}

	timestamp := backupTime.Unix()

	key := fmt.Sprintf("worlds/%s/%s.%d.tar.lzo", s.Id, s.Id, timestamp)

	url = fmt.Sprintf("https://%s.s3.amazonaws.com/%s", bucket, key)

	archiveDirBin, _ := filepath.Abs("bin/archive-dir")
	backupPaths, err := s.backupPaths()
	if err != nil {
		return
	}

	args := []string{url}
	for _, a := range backupPaths {
		args = append(args, a)
	}

	cmd := exec.Command(archiveDirBin, args...)
	cmd.Dir = s.workingPath()

	err = execWithOutput(cmd, os.Stdout, os.Stderr)

	return
}

func (s *Server) Compile(buildPath, cachePath string) (err error) {
	if fileExists(s.compilePath()) {
		cmd := exec.Command(s.compilePath(), buildPath, cachePath)
		err = execWithOutput(cmd, os.Stdout, os.Stderr)
	}

	return
}

func restoreDir(source string, dest string) error {
	var cmd *exec.Cmd

	if strings.Contains(source, "http") {
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
	return execWithOutput(cmd, os.Stdout, os.Stderr)
}

func (s *Server) funpackExec(dir string, script string, args ...string) (output []byte, err error) {
	scriptPath := s.funpackPath("bin", script)

	cmd := exec.Command("bundle", "exec", scriptPath)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "BUNDLE_GEMFILE="+s.funpackPath("Gemfile"))
	cmd.Dir = s.workingPath()

	return cmd.Output()
}

func (s *Server) backupPaths() ([]string, error) {
	info, err := s.funpackExec(s.workingPath(), "import")
	if err != nil {
		plog.Error(err, map[string]interface{}{
			"event":  "funpack_import_failed",
			"output": info,
		})
		return []string{}, err
	}

	var bi BackupInfo
	err = json.Unmarshal(info, &bi)
	if err != nil {
		return []string{}, err
	}

	return bi.Paths, nil
}

func (s *Server) funpackPath(paths ...string) string {
	args := []string{s.Path, "funpack"}
	for _, path := range paths {
		args = append(args, path)
	}
	return filepath.Join(args...)
}

func (s *Server) workingPath() string {
	return filepath.Join(s.Path, "working")
}

func (s *Server) compilePath() string {
	return s.funpackPath("bin", "compile")
}
