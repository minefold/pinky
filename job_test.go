package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

var data = []byte(`{
  "name": "start",
  "serverId": "1234",
  "funpack": "minecraft-essentials",
  "ram": {
    "min": 1024,
    "max": 1024
  },
  "settings" : {
    "banned": ["atnan"],
    "game_mode": 1,
    "new_player_can_build" : false,
    "ops": ["chrislloyd"],
    "seed": 123456789,
    "spawn_animals": true,
    "spawn_monsters": true,
    "whitelisted": ["whatupdave"]
  }
}`)

func TestServerJson(t *testing.T) {
	var job Job
	json.Unmarshal(data, &job)

	// settingsJson, _ := job.Settings.MarshalJSON()
	server := ServerSettings{
		Id:       job.ServerId,
		Funpack:  job.Funpack,
		Port:     4032,
		Ram:      job.Ram,
		Settings: job.Settings,
	}
	serverJson, _ := json.Marshal(server)

	expected := `{"id":"1234","funpack":"minecraft-essentials","port":4032,"ram":{"min":1024,"max":1024},"settings":{"banned":["atnan"],"game_mode":1,"new_player_can_build":false,"ops":["chrislloyd"],"seed":1.23456789e+08,"spawn_animals":true,"spawn_monsters":true,"whitelisted":["whatupdave"]}}`

	if string(serverJson) != expected {
		t.Error("expected", expected, "was", string(serverJson))
	}
}

func TestJobPopper(t *testing.T) {
	client := NewRedisConnection()
	defer client.Quit()
	client.Del("test:in")

	queue := NewJobPopper("test:in")

	jobs := make([]Job, 0)

	go func() {
		for job := range queue.C {
			jobs = append(jobs, job)
		}
		fmt.Println("should be 1 & 2:", jobs)
	}()

	client.Lpush("test:in", `{"name":"test","msg":"1"}`)
	client.Lpush("test:in", `{"name":"test","msg":"2"}`)

	time.Sleep(1 * time.Second)

	queue.Stop()

	time.Sleep(5 * time.Second)

	client.Lpush("test:in", `{"name":"test","msg":"3"}`)
	jobs = make([]Job, 0)
	go func() {
		for job := range queue.C {
			jobs = append(jobs, job)
		}
		fmt.Println("should be empty:", jobs)
	}()

	time.Sleep(5 * time.Second)
}
