package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Creates and changes into dir, executes work, changes back
func Chmkdir(dir string, work func()) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	err = os.MkdirAll(dir, 0777)
	if err != nil {
		return err
	}

	err = os.Chdir(dir)
	if err != nil {
		return err
	}
	defer os.Chdir(cwd)

	work()

	return nil
}

func setupTestPack() string {
	root := filepath.Join(os.TempDir(), "test-server")
	exec.Command("rm", "-rf", root).Run()
	cwd, _ := os.Getwd()
	packSrc := filepath.Join(cwd, "tests", "testpack")

	Chmkdir(root, func() {
		exec.Command("mkdir", "-p", "working").Run()
		exec.Command("cp", "-R", packSrc, "funpack").Run()
	})

	return root
}

func TestStartAndStop(t *testing.T) {
	plog = NewLog(map[string]interface{}{})
	root := setupTestPack()

	s := &Server{
		Id:    "test-server",
		Path:  root,
		Log:   plog,
		State: "starting",
	}

	_, err := s.StartServerProcess(filepath.Join(root, "server.pid"))
	if err != nil {
		t.Fatal(err)
	}

	events, err := s.Monitor()
	if err != nil {
		t.Fatal(err)
	}

	ev := <-events

	if ev.Event != "started" {
		t.Errorf("want %q, got %q", "started", ev.Event)
	}

	s.Stop()
}
