package main

import (
	// "bufio"
	"encoding/json"
	"fmt"
	// "log"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	// "time"
	"github.com/alphazero/Go-Redis"
)

type Job struct {
	Name     string
	ServerId string
	Funpack  string
	Ram      RamAllocation
	Settings interface{}
}

type RamAllocation struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

/*
	Global Redis storage
	(string) state/{serverId}  [starting|up|stopping]

	Local Redis storage
	TODO support TCP/UDP port ranges
	(set)  ports/{boxId}     [4001|4002]
*/

var redisClient redis.Client

var servers = map[string]*Server{}

func popRedisQueue(c chan Job, queue string) {
	client := createRedisClient()
	for {
		bytes, e := client.Brpop(queue, 60)
		if e != nil {
			// TODO figure out if this is a real error
			// most commonly it's the pop timing out
		}

		// If the pop times out, it just returns the key, no value
		if len(bytes) > 1 {
			// fmt.Sprintf("%s", bytes[1])
			var job Job
			json.Unmarshal(bytes[1], &job)

			fmt.Println(string(bytes[1]), job)

			c <- job
		}
	}
}

func fatal(err error) {
	fmt.Println(err, debug.Stack())
	panic(err)
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

		// TODO reserve an unused port
		port := 4032

		server.PrepareServerPath(serverPath)
		server.DownloadWorld(job.ServerId, serverPath)
		server.DownloadFunpack(job.Funpack, serverPath)
		server.WriteSettingsFile(serverPath,
			pidFile,
			port,
			job.ServerId,
			job.Funpack,
			job.Ram,
			job.Settings)
		server.StartServerProcess(serverPath,
			pidFile,
			job.ServerId)

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
		server := servers[job.ServerId]
		if server == nil {
			panic(fmt.Sprintf("no server for %s", job.ServerId))
		}
		server.Stop()
	} else {
		fmt.Println("Ignoring stop request")
	}
}

func createRedisClient() (client redis.Client) {
	// TODO add real connection info
	spec := redis.DefaultSpec()
	client, err := redis.NewSynchClientWithSpec(spec)
	if err != nil {
		panic("failed to create the client")
	}
	return
}

func handleRedisError(err error) {
	// TODO something more sane
	if err != nil {
		panic(err)
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

func isAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// returns serverId, pid
func readPidFromFile(pidFile string) (string, int) {
	b, err := ioutil.ReadFile(pidFile)
	if err != nil {
		fatal(err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		fatal(err)
	}

	parts := strings.Split(filepath.Base(pidFile), ".")
	fmt.Println(parts[0])

	return parts[0], pid
}

func discoverRunningServers(serverRoot string) {
	matches, err := filepath.Glob(filepath.Join(serverRoot, "*.pid"))
	if err != nil {
		fatal(err)
	}
	for _, pidFile := range matches {
		fmt.Println(fmt.Sprintf("found pid_file=%s", pidFile))
		serverId, pid := readPidFromFile(pidFile)

		if isAlive(pid) {
			fmt.Println("found running server", serverId, "pid", pid)

			server := new(Server)
			server.Id = serverId
			server.Path = filepath.Join(serverRoot, server.Id)

			servers[server.Id] = AttachServer(
				serverId,
				filepath.Join(serverRoot, server.Id),
				pid)

		} else {
			fmt.Println("found dead process", pid)
		}
	}
}

func main() {
	boxId := os.Args[1]

	redisClient = createRedisClient()

	serverRoot, _ := filepath.Abs("tmp/servers")
	exec.Command("mkdir", "-p", serverRoot).Run()

	discoverRunningServers(serverRoot)

	jobChannel := make(chan Job)
	boxQueueKey := fmt.Sprintf("jobs/%s", boxId)
	go popRedisQueue(jobChannel, boxQueueKey)

	fmt.Println("processing queue:", boxQueueKey)
	processJobs(jobChannel, serverRoot)

	/*
	   TODO figure out what the current state is
	   ie. find out what servers are currently running, update state, monitor etc.
	*/

}
