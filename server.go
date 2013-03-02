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
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Server struct {
	Id        string
	Path      string
	Pid       int
	Port      int
	HasWorld  bool
	Proc      *ManagedProcess
	Log       DataLogger
	State     string
	FunpackId string
}

type ServerSettings struct {
	Id        string        `json:"id"`
	FunpackId string        `json:"funpackId"`
	Funpack   string        `json:"funpack"`
	Port      int           `json:"port"`
	Ram       RamAllocation `json:"ram"`
	Data      *string       `json:"settings"`
}

type BackupInfo struct {
	Paths []string `json:"paths"`
}

func AttachServer(id, path string, pid, port int) (*Server, error) {
	settings, err := ReadServerSettings(path)
	if err != nil {
		return nil, err
	}

	s := &Server{Id: id, Path: path, Pid: pid, Port: port}
	s.Proc = NewManagedProcess(pid)
	s.Log = NewLog(map[string]interface{}{
		"serverId": id,
	})
	s.HasWorld = fileExists(s.funpackPath("bin", "import"))
	s.FunpackId = settings.FunpackId
	s.State = "up"
	return s, nil
}

func (s *Server) WriteSettingsFile(
	funpackId string,
	funpackUrl string,
	ram RamAllocation,
	data string) error {

	server := ServerSettings{
		Id:      s.Id,
		Funpack: funpackUrl,
		Port:    s.Port,
		Ram:     ram,
		Data:    &data,
	}
	serverJson, err := json.Marshal(server)
	if err != nil {
		return err
	}

	ioutil.WriteFile(s.SettingsFile(), serverJson, 0644)
	return nil
}

func ReadServerSettings(path string) (*ServerSettings, error) {
	settingsFile := filepath.Join(path, "server.json")
	b, err := ioutil.ReadFile(settingsFile)
	if err != nil {
		return nil, err
	}

	var settings *ServerSettings
	err = json.Unmarshal(b, &settings)
	return settings, err
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
		event, parseErr := ParseServerEvent(line)
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
			switch event.Type() {
			case "stopping":
				go s.EnsureServerStopped()
			}
			eventsOut <- event
		}

		close(eventsOut)
	}()

	return eventsOut, nil
}

func waitFor(duration time.Duration, test func() bool) bool {
	t := time.NewTicker(1 * time.Second)
	started := time.Now()

	for _ = range t.C {
		if test() {
			return true
		}
		if time.Since(started) > 60*time.Second {
			break
		}
	}

	return false
}

func (s *Server) EnsureServerStopped() {
	if s.Proc.WaitForExit(60 * time.Second) {
		return
	}

	// didn't stop after 1 minute, send TERM
	s.Log.Info(map[string]interface{}{
		"event":  "ensure_stop_timeout",
		"pid":    s.Pid,
		"action": "TERM",
	})
	s.Kill(syscall.SIGTERM)
	if s.Proc.WaitForExit(60 * time.Second) {
		return
	}

	// didn't stop after 1 minute, send KILL until exit
	for {
		s.Log.Info(map[string]interface{}{
			"event":  "ensure_stop_timeout",
			"pid":    s.Pid,
			"action": "KILL",
		})
		s.Kill(syscall.SIGKILL)
		if !s.Proc.IsRunning() {
			s.Log.Info(map[string]interface{}{
				"event":   "ensure_stop_finished",
				"details": "exited with KILL",
				"pid":     s.Pid,
			})
			return
		}
		time.Sleep(5 * time.Second)
	}
}

func (s *Server) Stop() {
	// TODO this hangs if server is stopped
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
	syscall.Kill(s.Pid, sig)
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

func (s *Server) WriteDataFile(data string) error {
	return ioutil.WriteFile(s.DataFile(), []byte(data), 0644)
}

func (s *Server) StartServerProcess(pidFile string) (pid int, err error) {
	// TODO run Procfile instead of bin/run
	command := filepath.Join(s.Path, "funpack", "bin", "run")
	bufferCmd, _ := filepath.Abs("bin/buffer-process")

	cmd := exec.Command(bufferCmd,
		"-d", s.Path,
		"-p", pidFile,
		"-l", filepath.Join(s.Path, "buffer.log"),
		command,
		s.SettingsFile()) // TODO: don't pass settings file

	s.Log.Info(map[string]interface{}{
		"event":      "starting_server",
		"workingDir": s.workingPath(),
		"pidFile":    pidFile,
		"dataFile":   s.DataFile(),
	})

	// TODO: only send ENV vars the funpack needs
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "BUNDLE_GEMFILE="+s.funpackPath("Gemfile"))
	cmd.Env = append(cmd.Env, "SHARED_DIR="+s.funpackBuildPath())
	cmd.Env = append(cmd.Env, "DATAFILE="+s.DataFile())
	cmd.Env = append(cmd.Env, "PORT="+strconv.Itoa(s.Port))
	cmd.Dir = s.workingPath()

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

	s.Proc = NewManagedProcess(s.Pid)
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

func (s *Server) DownloadFunpack(funpackId string, funpackUrl string, testWorld bool) error {
	err := restoreDir(funpackUrl, s.funpackPath())
	if err != nil {
		return err
	}

	s.HasWorld = fileExists(s.funpackPath("bin", "import"))
	s.FunpackId = funpackId

	plog.Info(map[string]interface{}{
		"event": "Gemfile detected",
	})

	if fileExists(s.funpackPath("Gemfile")) {
		cmd := exec.Command("bundle", "install", "--deployment")
		cmd.Dir = s.funpackPath()
		output, err := cmd.CombinedOutput()
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":  "bundle_install_failed",
				"output": string(output),
			})
			return err
		}

		if testWorld {
			// Test the import script now. If it fails later we'll fail to
			// backup the world
			_, err = s.backupPaths()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) DownloadWorld(world string) error {
	return restoreDir(world, s.workingPath())
}

func (s *Server) BackupWorld(backupTime time.Time) (url string, size int64, err error) {
	if !s.HasWorld {
		return "", 0, errors.New("Nothing to back up")
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
	out, err := cmd.Output()

	size, _ = strconv.ParseInt(strings.TrimSpace(string(out)), 0, 0)

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

func (s *Server) funpackCmd(bin string, args ...string) *exec.Cmd {
	scriptPath := s.funpackPath("bin", bin)

	cmd := exec.Command("bundle", "exec", scriptPath)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "BUNDLE_GEMFILE="+s.funpackPath("Gemfile"))
	cmd.Env = append(cmd.Env, "BUILD_DIR="+s.funpackBuildPath())
	cmd.Dir = s.workingPath()

	return cmd
}

func (s *Server) backupPaths() ([]string, error) {
	info, err := s.funpackCmd("import").CombinedOutput()
	if err != nil {
		plog.Error(err, map[string]interface{}{
			"event":  "funpack_import_failed",
			"output": string(info),
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

func (s *Server) DataFile() string {
	return filepath.Join(s.Path, "data.json")
}

func (s *Server) SettingsFile() string {
	return filepath.Join(s.Path, "server.json")
}

func (s *Server) funpackBuildPath() string {
	basePath := os.Getenv("FUNPACKS_PATH")
	if basePath == "" {
		basePath = "/usr/local/funpacks"
	}
	return filepath.Join(basePath, s.FunpackId, "build")
}
