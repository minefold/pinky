package main

import (
	"encoding/json"
	"testing"
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

	expected := `{"id":"1234","funpack":"minecraft-essentials","port":4032,"ram":{"Min":1024,"Max":1024},"settings":{"banned":["atnan"],"game_mode":1,"new_player_can_build":false,"ops":["chrislloyd"],"seed":1.23456789e+08,"spawn_animals":true,"spawn_monsters":true,"whitelisted":["whatupdave"]}}`

	if string(serverJson) != expected {
		t.Error("expected", expected, "was", string(serverJson))
	}
}
