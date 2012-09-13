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
	Name     string
	ServerId string
	WorldId  string
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

/*
	Global Redis storage
	(string) state/{serverId}  [starting|up|stopping]

	Local Redis storage
	(set)  ports/{boxId}     [4001|4002]
*/

var redisClient redis.Client

// TODO store in redis
var servers = map[string]*Server{}
var state = map[string]string{}
var ports = map[int]bool{}

func popRedisQueue(c chan Job, queue string) {
	client := createRedisClient()
	for {
		bytes, e := client.Brpop(queue, 20)
		if e != nil {
			log.Println("error on BRPOP", e)
			return
		}

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
	if attemptStartingTransition(job.ServerId) {

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

		for event := range events {
			fmt.Println(event)
			switch event.Event {
			case "started":
				transitionStartingToUp(job.ServerId)
			case "stopping":
				attemptStoppingTransition(job.ServerId)
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
	if attemptStoppingTransition(job.ServerId) {
		servers[job.ServerId].Stop()
	} else {
		fmt.Println("Ignoring stop request")
	}
}

func createRedisClient() (client redis.Client) {
	// TODO add real connection info
	spec := redis.DefaultSpec().Db(13)
	client, err := redis.NewSynchClientWithSpec(spec)
	if err != nil {
		panic("failed to create the client")
	}
	return
}

func handleRedisError(err error) {
	// TODO something more sane
	if err != nil {
		panic("redis connection error")
	}
}

func stateKey(serverId string) string {
	return fmt.Sprintf("state/%s", serverId)
}

func stateTransition(serverId string, from string, to string, enforceStartingCondition bool) bool {
	// TODO race condition?
	serverStateKey := stateKey(serverId)

	oldValue, err := redisClient.Get(serverStateKey)
	handleRedisError(err)

	if string(oldValue) != from {
		if enforceStartingCondition {
			panic(
				fmt.Sprintf(
					"invalid state! Expected %s was %s", from, oldValue))
		} else {
			return false
		}
	}

	if to != "" {
		err := redisClient.Set(serverStateKey, []byte(to))
		handleRedisError(err)
	} else {
		_, err := redisClient.Del(serverStateKey)
		handleRedisError(err)
	}

	return true
}

func attemptStartingTransition(serverId string) bool {
	return stateTransition(serverId, "", "starting", false)
}

func attemptStoppingTransition(serverId string) bool {
	return stateTransition(serverId, "up", "stopping", false)
}

func transitionStartingToUp(serverId string) {
	stateTransition(serverId, "starting", "up", true)
}

func transitionUpToStopping(serverId string) {
	stateTransition(serverId, "up", "stopping", true)
}

func transitionStoppingToStopped(serverId string) {
	stateTransition(serverId, "stopping", "", true)
}

func processJobs(jobChannel chan Job, serverRoot string) {
	for {
		job := <-jobChannel

		fmt.Println(job)

		switch job.Name {
		case "start":
			go startServer(job, serverRoot)
		case "stop":
			go stopServer(job)
		default:
			fmt.Println("Unknown job", job)
		}
	}
}

func main() {
	boxId := os.Args[1]

	redisClient = createRedisClient()

	serverRoot, _ := filepath.Abs("tmp/servers")
	exec.Command("mkdir", "-p", serverRoot).Run()

	jobChannel := make(chan Job)
	boxQueueKey := fmt.Sprintf("box/%s/queue", boxId)
	go popRedisQueue(jobChannel, boxQueueKey)

	processJobs(jobChannel, serverRoot)

	fmt.Println("processing queue:", boxQueueKey)

	/*
	   TODO figure out what the current state is
	   ie. find out what servers are currently running, update state, monitor etc.
	*/

}
