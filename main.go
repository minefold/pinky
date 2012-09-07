package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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

/worlds
/worlds/1
/worlds/1/funpack
/worlds/1/working
/worlds/1/backup

id.port.pid

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
		fmt.Println("Starting world", job.ServerId)
		
		serverPath := filepath.Join(serverRoot, job.ServerId)
		pidFile := filepath.Join(pidRoot, fmt.Sprintf("%s.pid", job.ServerId))
		
		fmt.Println(serverPath)

		prepareServerPath(serverPath)
		downloadFunpack(job.WorldId, serverPath)
		downloadWorld(job.WorldId, serverPath)
		startServerProcess(serverPath, pidFile)
		// transitionStartingToUp(job.ServerId)
		go monitor(serverPath)

	} else {
		fmt.Println("Ignoring start request")
	}
}

func monitor(serverPath string) {
	stdoutPipe := filepath.Join(serverPath, "pipe_stdout")
	
	// wait for file to exist
	time.Sleep(3 * time.Second)
	stdout, err := os.OpenFile(stdoutPipe, syscall.O_NONBLOCK | syscall.O_RDONLY, 0x0666)
	if err != nil {
		panic(err)
	}
	defer stdout.Close()

	r := bufio.NewReader(stdout)
	line, isPrefix, err := r.ReadLine()
    for err == nil && !isPrefix {
        s := string(line)
        fmt.Println(s)
        line, isPrefix, err = r.ReadLine()
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

func processJobs(jobChannel chan Job) {
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
	
	processJobs(jobChannel)

  /*
    figure out what the current state is
    ie. find out what servers are currently running, update state, monitor etc.
  */

  /*
    process redis queue
  */



}