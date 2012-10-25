package main

import (
	"os/exec"
	"strings"
)

// For monitoring Work In Progress
// we use this so we can shut down pinky gracefully when
// all current work is completed

type WipGenerator struct {
	C     chan chan bool
	Count int
}

func NewWipGenerator() *WipGenerator {
	workInProgress := map[string]string{}

	wipGen := make(chan chan bool)

	w := &WipGenerator{
		C:     wipGen,
		Count: 0,
	}

	go func() {
		for {
			id := uuid()
			finished := make(chan bool)
			wipGen <- finished
			workInProgress[id] = id
			w.Count = len(workInProgress)

			go func() {
				<-finished
				delete(workInProgress, id)
				w.Count = len(workInProgress)
			}()
		}
	}()

	return w
}

func uuid() string {
	id, _ := exec.Command("uuidgen").Output()
	return strings.TrimSpace(string(id))
}
