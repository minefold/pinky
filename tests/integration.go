package main

import (
	"fmt"
	"github.com/simonz05/godis/redis"
	"labix.org/v2/mgo/bson"
	"os"
	"strconv"
	"time"
)

var r *redis.Client

func watchServers(liveServersCount chan int, upServersCount chan int) {
	prevLive := 0
	prevUp := 0
	for {
		keys, _ := r.Keys("server/state/*")

		liveServers := len(keys)
		upServers := 0

		for _, key := range keys {
			state := redisGet(r, key)

			if string(state) == "up" {
				upServers++
			}
		}

		if prevLive != liveServers {
			prevLive = liveServers
			liveServersCount <- liveServers
		}

		if prevUp != upServers {
			prevUp = upServers
			upServersCount <- upServers
		}

		time.Sleep(1 * time.Second)
	}
}

func startServer(serverId string) {
	r.Lpush("jobs/1", []byte(fmt.Sprintf(`{
		"name":"start",
		"serverId":"%s",
		"funpack":"/home/vagrant/funpacks/minecraft-vanilla/",
		"ram": { "min": 512, "max": 512  },
		"settings": {}
	}`, serverId)))
}

func stopServer(serverId string) {
	r.Lpush("jobs/1", []byte(fmt.Sprintf(`{
		"name":"stop",
		"serverId":"%s"}`, serverId)))
}

func countServersTo(serverCount chan int, target int) {
	for count := range serverCount {
		fmt.Println(count, "servers")
		if count == target {
			return
		}
	}
}

func main() {
	maxServers := 100
	if len(os.Args) > 1 {
		maxServers, _ = strconv.Atoi(os.Args[1])
	}

	r = redis.New("", 0, "")

	upServerCount := make(chan int, maxServers*10)
	liveServerCount := make(chan int, maxServers*10)
	go watchServers(liveServerCount, upServerCount)

	serverIds := make([]string, maxServers)

	for i := 0; i < maxServers; i++ {
		serverIds[i] = bson.NewObjectId().Hex()
	}

	for i := 0; i < maxServers; i++ {
		fmt.Println(serverIds[i])
		startServer(serverIds[i])
		time.Sleep(200 * time.Millisecond)
	}

	countServersTo(upServerCount, maxServers)
	fmt.Println("up")

	for i := 0; i < maxServers; i++ {
		stopServer(serverIds[i])
	}

	countServersTo(liveServerCount, 0)
}

func retry(maxRetries int, delay time.Duration, work func() error) error {
	err := work()
	retries := 0
	for err != nil && retries < maxRetries {
		time.Sleep(delay)
		err = work()
		retries += 1
	}
	return err
}

func redisGet(redisClient *redis.Client, key string) []byte {
	var result []byte
	err := retry(50, 100*time.Millisecond, func() error {
		var err error
		result, err = redisClient.Get(key)
		if err != nil {
			fmt.Println("redis retry...")
		}
		return err
	})
	if err != nil {
		panic(err)
	}
	return result
}
