package main

import (
	"encoding/json"
)

// (start|stop|broadcast|tell|multi)
type Job struct {
	Name       string
	ServerId   string
	FunpackId  string
	FunpackUrl string
	Ram        RamAllocation
	SnapshotId string
	WorldUrl   string
	Settings   interface{}

	// for broadcast
	Msg string

	// for tell & kick
	Username string
}

type JobPopper struct {
	C       chan Job
	stopped bool
}

func NewJobPopper(name string) *JobPopper {
	p := &JobPopper{
		C: make(chan Job),
	}
	go p.process(name)
	return p
}

func (p *JobPopper) Stop() {
	p.stopped = true
}

func (p *JobPopper) process(name string) {
	client := NewRedisConnection()
	defer client.Quit()
	defer close(p.C)

	for !p.stopped {
		reply, e := client.Brpop([]string{name}, 5)
		if e == nil {
			val := reply.BytesArray()[1]

			var job Job
			json.Unmarshal(val, &job)

			p.C <- job
		}
	}
}
