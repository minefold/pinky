package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"
	"github.com/alphazero/Go-Redis"
)

type Job struct {
	Name string
	ServerId string
	WorldId string
}

/*
	Funpack Methods
*/

func downloadFunpack(id string, dest string) {
	fmt.Println("downloading funpack", id, dest)
	time.Sleep(5 * time.Second)
	fmt.Println("downloaded funpack", id, dest)
}

func downloadWorld(id string, dest string) {
	fmt.Println("downloading world", id, dest)
	time.Sleep(5 * time.Second)
	fmt.Println("downloaded world", id, dest)
}

func backupWorld(id string, dest string) {
	fmt.Println("backing up world", id, dest)
	time.Sleep(5 * time.Second)
	fmt.Println("backed up world", id, dest)
}

func popRedisQueue(c chan Job, client redis.Client, queue string) {
	for {
		bytes, e := client.Brpop(queue, 20);
		if e!= nil { log.Println ("error on BRPOP", e); return }
		
		// If the pop times out, it just returns the key, no value
		if len(bytes) > 1 {
			// fmt.Sprintf("%s", bytes[1])
			var job Job
			json.Unmarshal(bytes[1], &job)
			c <- job
		}
	}
}

func startServerProcess(dest string) {
	startCmd := fmt.Sprintf("%s/bin/start", dest)
	fmt.Println("starting", startCmd)
}

func startWorld(job Job, dest string) {
	if (attemptStartingTransition(job.ServerId)) {
		fmt.Println("Starting world", job.ServerId)
		
		downloadFunpack(job.WorldId, dest)
		downloadWorld(job.WorldId, dest)
		startServerProcess(dest)
		transitionStartingToUp(job.ServerId)
		// monitor(job.ServerId)

	} else {
		fmt.Println("Ignoring start request")
	}
	
}

func stopWorld(job Job) {
	fmt.Println("Stopping world")
}

func createRedisClient(database int) (client redis.Client) {
	spec := redis.DefaultSpec().Db(database)
	client, e := redis.NewSynchClientWithSpec(spec);
	if e != nil { 
		panic("failed to create the client") 
	}
	return
}

var state = map[string] string {}

func attemptStartingTransition(serverId string) bool {
	// redis: state/1
	
	if state[serverId] == "" {
		state[serverId] = "starting"
		return true
	} else {
		return false
	}
	// what the shit!?
	return true
}

func transitionStartingToUp(serverId string) {
	if state[serverId] == "starting" {
		state[serverId] = "up"
	} else {
		panic(
			fmt.Sprintf("invalid state! Expected starting was %s", state[serverId]))
	}	
}

func releaseServerLock(serverId string) {
	delete(state, serverId)
}

func main() {
	boxId := os.Args[1]
	
	worldPath := "/usr/bin/something"

	client := createRedisClient(13)
	
	boxQueueKey := fmt.Sprintf("box/%s/queue", boxId)
	fmt.Println("processing queue:", boxQueueKey)
	
	jobChannel := make(chan Job)

	go popRedisQueue(jobChannel, client, boxQueueKey)
	
	for {
		job := <- jobChannel
		
		fmt.Println(job)
		
		switch job.Name {
		case "start": go startWorld(job, worldPath)
		case "stop": go stopWorld(job)
		default: fmt.Println("Unknown job", job)
		}
	}
	

  /*
    figure out what the current state is
    ie. find out what servers are currently running, update state, monitor etc.
  */

  /*
    process redis queue
  */



}