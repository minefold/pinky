package main

import (
	"fmt"
	"github.com/kristiankristensen/Go-Redis"
	"labix.org/v2/mgo/bson"
	"strings"
	"time"
)

var r redis.Client

func createRedisClient() (client redis.Client) {
	spec := redis.DefaultSpec()
	client, err := redis.NewSynchClientWithSpec(spec)
	if err != nil {
		panic("failed to create the client")
	}
	return
}

func watchServers(serverCount chan int) {
	prev := 0
	for {
		keys, _ := r.Keys("server/state/*")

		upServers := 0

		for _, key := range keys {
			serverId := strings.Split(key, "/")[2]

			state, err := r.Get(key)
			if err != nil {
				panic(err)
			}

			fmt.Println(key, serverId, string(state))

			if string(state) == "up" {
				upServers++
			}
		}

		if prev != upServers {
			prev = upServers
			serverCount <- upServers
		}

		time.Sleep(1 * time.Second)
	}
}

func startServer(serverId string) {
	r.Lpush("jobs/1", []byte(fmt.Sprintf(`{
		"name":"start",
		"serverId":"%s",
		"funpack":"minecraft-essentials",
		"ram": { "min": 512, "max": 512  },
		"settings": { 
			"banned": ["atnan"], 
			"game_mode": 1, 
			"ops": ["chrislloyd"],
			"seed": 123456789,
			"spawn_animals": true,
			"spawn_monsters": true,
			"whitelisted": ["whatupdave"]  }}`, serverId)))
}

func main() {
	r = createRedisClient()

	serverCount := make(chan int)
	go watchServers(serverCount)

	startServer(bson.NewObjectId().Hex())

	for count := range serverCount {
		fmt.Println(count, "servers")
	}

	// lpush jobs/1 "{\"name\":\"stop\",\"serverId\":\"508227b5474a80599bcab3aa\"}"

}
