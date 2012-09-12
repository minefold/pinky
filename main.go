package main

import (
	// "bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	// "time"
	"github.com/alphazero/Go-Redis"
)

type Job struct {
	Name string
	ServerId string
	WorldId string
}

/*
	Funpack Methods
		
	/servers
	/servers/1.pid
	
	/servers/1/stdin
	/servers/1/stderr
	/servers/1/stdout
	
	/servers/1/funpack/
	/servers/1/working/
	/servers/1/backup/

*/

var servers = map[string] *Server {}

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

func startServer(job Job, serverRoot string) {
	if (attemptStartingTransition(job.ServerId)) {
		
		server := new(Server)
		server.Id = job.ServerId
		server.Path = filepath.Join(serverRoot, server.Id)
		
		servers[server.Id] = server
		
		fmt.Println("Starting world", job.ServerId)
		
		serverPath := server.Path
		pidFile := filepath.Join(serverRoot, fmt.Sprintf("%s.pid", job.ServerId))
		
		server.PrepareServerPath(serverPath)
		server.DownloadFunpack(job.WorldId, serverPath)
		server.DownloadWorld(job.WorldId, serverPath)
		server.StartServerProcess(serverPath, pidFile)
		
		events := make(chan ServerEvent)
		
		go server.Monitor(events)
		
		for event := range(events) {
			fmt.Println(event)
			switch event.Event {
				case "started": transitionStartingToUp(job.ServerId)
				case "stopping": attemptStoppingTransition(job.ServerId)
			}
		}
		transitionStoppingToStopped(job.ServerId)
		fmt.Println("server stopped")
		exec.Command("rm", "-f", pidFile).Run()
		delete(servers, job.ServerId)
	} else {
		fmt.Println("Ignoring start request")
	}
}

func stopServer(job Job) {
	if (attemptStoppingTransition(job.ServerId)) {
		servers[job.ServerId].Stop()
	} else {
		fmt.Println("Ignoring stop request")
	}
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

func attemptStoppingTransition(serverId string) bool {
	if servers[serverId] == nil {
		return false
	}
	
	if state[serverId] == "up" {
		state[serverId] = "stopping"
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

func transitionUpToStopping(serverId string) {
	fmt.Println("STOPZ!")
	if state[serverId] == "up" {
		state[serverId] = "stopping"
	} else {
		panic(
			fmt.Sprintf("invalid state! Expected up was %s", state[serverId]))
	}	
}

func transitionStoppingToStopped(serverId string) {
	if state[serverId] == "stopping" {
		delete(state, serverId)
	} else {
		panic(
			fmt.Sprintf("invalid state! Expected starting was %s", state[serverId]))
	}	
}

func processJobs(jobChannel chan Job, serverRoot string) {
	for {
		job := <- jobChannel
		
		fmt.Println(job)
		
		switch job.Name {
		case "start": go startServer(job, serverRoot)
		case "stop": go stopServer(job)
		default: fmt.Println("Unknown job", job)
		}
	}
}

func main() {
	boxId := os.Args[1]
	
	serverRoot, _ := filepath.Abs("tmp/servers")
	
	exec.Command("mkdir", "-p", serverRoot).Run()

	client := createRedisClient(13)
	
	boxQueueKey := fmt.Sprintf("box/%s/queue", boxId)
	fmt.Println("processing queue:", boxQueueKey)
	
	jobChannel := make(chan Job)

	go popRedisQueue(jobChannel, client, boxQueueKey)
	
	processJobs(jobChannel, serverRoot)

  /*
    figure out what the current state is
    ie. find out what servers are currently running, update state, monitor etc.
  */

  /*
    process redis queue
  */



}