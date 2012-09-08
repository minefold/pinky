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
	
	
	/pids/1.pid
	
	/servers
	/servers/1
	
	/servers/1/stdin
	/servers/1/stderr
	/servers/1/stdout
	
	/servers/1/funpack/
	/servers/1/working/
	/servers/1/backup/

*/

func downloadFunpack(id string, dest string) {
	fmt.Println("downloading funpack", id, dest)
	
	funpackPath := filepath.Join(dest, "funpack")
	
	exec.Command("cp", "-r", 
		"/Users/dave/code/minefold/funpacks/dummy.funpack", funpackPath).Run()
	
	fmt.Println("downloaded funpack", id, dest)
}

func downloadWorld(id string, dest string) {
	fmt.Println("downloading world", id, dest)
	fmt.Println("downloaded world", id, dest)
}

func backupWorld(id string, dest string) {
	fmt.Println("backing up world", id, dest)
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

func startServerProcess(dest string, pidFile string) {
	command := filepath.Join(dest, "funpack", "bin", "start")
	workingDirectory := filepath.Join(dest, "working")
	
	bufferCmd, _ := filepath.Abs("bin/buffer-process")
	
	fmt.Println("starting", command)
	cmd := exec.Command(bufferCmd, "-d", dest, "-p", pidFile, command)
	cmd.Dir = workingDirectory
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		panic(err)
	}
	
	err = cmd.Start()
	if err != nil {
		panic(err)
	}
	
	buf := make([]byte, 1024)
	_, err = stdout.Read(buf)
	if err != nil {
		panic(err)
	}
	
	fmt.Println(string(buf))
}

func prepareServerPath(serverPath string) {
	workingDirectory := filepath.Join(serverPath, "working")
	exec.Command("mkdir", "-p", workingDirectory).Run()
}

func startServer(job Job, serverRoot string, pidRoot string) {
	if (attemptStartingTransition(job.ServerId)) {
		
		server := new(Server)
		server.Id = job.ServerId
		server.Path = filepath.Join(serverRoot, server.Id)
		
		fmt.Println("Starting world", job.ServerId)
		
		serverPath := server.Path
		pidFile := filepath.Join(pidRoot, fmt.Sprintf("%s.pid", job.ServerId))
		
		fmt.Println(serverPath)

		prepareServerPath(serverPath)
		downloadFunpack(job.WorldId, serverPath)
		downloadWorld(job.WorldId, serverPath)
		startServerProcess(serverPath, pidFile)
		// transitionStartingToUp(job.ServerId)
		
		events := make(chan ServerEvent)
		
		go server.Monitor(events)
		
		for event := range(events) {
			fmt.Println("got event", event)
		}
		fmt.Println("All done!")

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

func processJobs(jobChannel chan Job, serverRoot string, pidRoot string) {
	for {
		job := <- jobChannel
		
		fmt.Println(job)
		
		switch job.Name {
		case "start": go startServer(job, serverRoot, pidRoot)
		case "stop": go stopWorld(job)
		default: fmt.Println("Unknown job", job)
		}
	}
}

func main() {
	boxId := os.Args[1]
	
	serverRoot, _ := filepath.Abs("tmp/servers")
	pidRoot, _ := filepath.Abs("tmp/pids")
	
	exec.Command("mkdir", "-p", pidRoot).Run()

	client := createRedisClient(13)
	
	boxQueueKey := fmt.Sprintf("box/%s/queue", boxId)
	fmt.Println("processing queue:", boxQueueKey)
	
	jobChannel := make(chan Job)

	go popRedisQueue(jobChannel, client, boxQueueKey)
	
	processJobs(jobChannel, serverRoot, pidRoot)

  /*
    figure out what the current state is
    ie. find out what servers are currently running, update state, monitor etc.
  */

  /*
    process redis queue
  */



}