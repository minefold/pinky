package main

import (
  	"fmt"
	"log"
  	"os"
	"encoding/json"
  	"github.com/alphazero/Go-Redis"
)

type Job struct {
	Name string
	ServerId string
}

func popRedisQueue(c chan Job, client redis.Client, queue string) {
	for {
		bytes, e := client.Brpop(queue, 20);
		if e!= nil { log.Println ("error on BRPOP", e); return }
		
		// If the pop times out, it just returns the key, no value
		if len(bytes) > 1 {
			// fmt.Sprintf("%s", bytes[1])
			var m Job
			json.Unmarshal(bytes[1], &m)
			c <- m
		}
	}
}

func startWorld(job Job) {
	fmt.Println("Starting world", job.ServerId)
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

func main() {
	boxId := os.Args[1]

	fmt.Println("box_id: ", boxId)

	client := createRedisClient(13)

	boxQueueKey := fmt.Sprintf("box/%s/queue", boxId)
	
	jobChannel := make(chan Job)

	go popRedisQueue(jobChannel, client, boxQueueKey)
	
	for {
		job := <- jobChannel
		fmt.Println(job)
	
		switch job.Name {
		case "start": startWorld(job)
		case "stop": stopWorld(job)
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