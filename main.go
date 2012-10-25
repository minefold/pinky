package main

import (
	"encoding/json"
	"fmt"
	// "log"
	"github.com/kristiankristensen/Go-Redis"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	// "runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Job struct {
	Name     string
	ServerId string
	Funpack  string
	Ram      RamAllocation
	World    string
	Settings interface{}
}

type RamAllocation struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type RedisServerInfo struct {
	Pid  int `json:"pid"`
	Port int `json:"port"`
}

/*
	Global Redis storage
	(string) server/state/{serverId}  [starting|up|stopping]

	Local Redis storage
	TODO support TCP/UDP port ranges
	(set)  ports/{boxId}     [4001|4002]
*/

var redisClient redis.Client
var boxId string
var servers = map[string]*Server{}
var serverRoot string
var wipGen *WipGenerator

func popRedisQueue(c chan Job, queue string) {
	client := createRedisClient()
	for {
		bytes, e := client.Brpop(queue, 60)
		if e != nil {
			// TODO figure out if this is a real error
			// most commonly it's the pop timing out
			// fmt.Println("redis error", e)
		}

		// If the pop times out, it just returns the key, no value
		if len(bytes) > 1 {
			// fmt.Sprintf("%s", bytes[1])
			var job Job
			json.Unmarshal(bytes[1], &job)

			fmt.Println(string(bytes[1]), job)

			c <- job
		} else {
			// fmt.Println("redis pop timed out?")
		}
	}
}

func serverPath(serverId string) string {
	return filepath.Join(serverRoot, serverId)
}

func pidFile(serverId string) string {
	return filepath.Join(serverRoot, fmt.Sprintf("%s.pid", serverId))
}

func startServer(job Job) {
	if attemptStartingTransition(job.ServerId) {

		server := new(Server)
		server.Id = job.ServerId
		server.Path = filepath.Join(serverRoot, server.Id)

		servers[server.Id] = server

		fmt.Println("Starting world", job.ServerId)

		pidFile := pidFile(job.ServerId)

		// TODO reserve an unused port
		port := 20000

		server.PrepareServerPath()
		server.DownloadWorld(job.World)
		server.DownloadFunpack(job.Funpack)
		server.WriteSettingsFile(
			port,
			job.Funpack,
			job.Ram,
			job.Settings)
		pid := server.StartServerProcess(pidFile)

		events := server.Monitor()
		go processServerEvents(job.ServerId, events)

		serverInfo := RedisServerInfo{Pid: pid, Port: port}
		serverJson, err := json.Marshal(serverInfo)
		if err != nil {
			panic(err)
		}
		redisClient.Set(
			fmt.Sprintf("pinky/%s/servers/%s", boxId, job.ServerId),
			serverJson)

	} else {
		fmt.Println("Ignoring start request")
	}
}

func processServerEvents(serverId string, events chan ServerEvent) {
	var stopWip chan bool
	for event := range events {
		fmt.Println(event)
		switch event.Event {
		case "started":
			transitionStartingToUp(serverId)
		case "stopping":
			if attemptStoppingTransition(serverId) {
				stopWip = <-wipGen.C
				fmt.Println("----- Starting Work 1")
			}
		}
	}

	if stopWip == nil {
		fmt.Println("----- Starting Work 2")
		stopWip = <-wipGen.C
	}

	server := servers[serverId]
	if server == nil {
		panic(fmt.Sprintf("no server for %s", serverId))
	}

	// TODO check if we need to do a backup
	// eg. the world didn't start properly or
	// this game has no persistent state
	backupTime := time.Now()
	key, err := server.BackupWorld(backupTime)
	if err != nil {
		// TODO retries, recovery etc
		panic(err)
	}

	err = storeBackupInMongo(serverId, key, backupTime)
	if err != nil {
		// TODO retries, recovery etc
		panic(err)
	}

	transitionToStopped(serverId)
	removeServerArtifacts(serverId)
	fmt.Println("server stopped")
	delete(servers, serverId)
	fmt.Println("----- Stopping Work 3")
	stopWip <- true
}

func stopServer(job Job) {
	if attemptStoppingTransition(job.ServerId) {
		fmt.Println("----- Starting Work 4")
		stopWip := <-wipGen.C

		server := servers[job.ServerId]
		if server == nil {
			panic(fmt.Sprintf("no server for %s", job.ServerId))
		}
		server.Stop()
		stopWip <- true
	} else {
		fmt.Println("Ignoring stop request")
	}
}

func removeServerArtifacts(serverId string) {
	exec.Command("rm", "-f", pidFile(serverId)).Run()
	exec.Command("rm", "-rf", serverPath(serverId)).Run()

	redisClient.Del(
		fmt.Sprintf("pinky/%s/servers/%s", boxId, serverId))
	redisClient.Del(
		fmt.Sprintf("server/state/%s", serverId))
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
	return fmt.Sprintf("server/state/%s", serverId)
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

func redisGet(key string) []byte {
	var result []byte
	err := retry(5, 100*time.Millisecond, func() error {
		var err error
		result, err = redisClient.Get(key)
		if err != nil {
			fmt.Println(err)
		}
		return err
	})
	handleRedisError(err)
	return result
}

func redisDel(key string) bool {
	var result bool
	err := retry(5, 100*time.Millisecond, func() error {
		var err error
		result, err = redisClient.Del(key)
		return err
	})
	handleRedisError(err)
	return result
}

func stateTransition(
	serverId string,
	from string,
	to string,
	enforceStartingCondition bool) bool {

	// TODO race condition?
	serverStateKey := stateKey(serverId)
	oldValue := redisGet(serverStateKey)

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

func transitionToStopped(serverId string) {
	stateTransition(serverId, "stopping", "", false)
}

func getState() []byte {
	key := fmt.Sprintf("pinky/%s/state", boxId)
	return redisGet(key)
}

func setStateTo(state string) {
	key := fmt.Sprintf("pinky/%s/state", boxId)
	redisClient.Set(key, []byte(state))
}

func processJobs(jobChannel chan Job) {
	for {
		job := <-jobChannel

		if string(getState()) == "down" {
			fmt.Println("ignoring job: pinky is down")
			continue
		}

		switch job.Name {

		case "start":
			go func() {
				wip := <-wipGen.C
				startServer(job)
				wip <- true
			}()

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
		panic(err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		panic(err)
	}

	parts := strings.Split(filepath.Base(pidFile), ".")

	return parts[0], pid
}

func discoverRunningServers() {
	matches, err := filepath.Glob(filepath.Join(serverRoot, "*.pid"))
	if err != nil {
		panic(err)
	}
	for _, pidFile := range matches {
		serverId, pid := readPidFromFile(pidFile)
		serverPath := serverPath(serverId)
		fmt.Println(
			fmt.Sprintf("found pid_file=%s path=%s", pidFile, serverPath))

		if isAlive(pid) {
			fmt.Println("found running server", serverId, "pid", pid)

			servers[serverId] = AttachServer(
				serverId,
				serverPath,
				pid)

			events := servers[serverId].Monitor()
			go processServerEvents(serverId, events)

		} else {
			fmt.Println("found dead process", pid)
			removeServerArtifacts(serverId)
		}
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			// TODO bugsnag
			fmt.Println("ERMAHGERD FERTEL ERRERRRR! ಠ_ಠ")
			panic(r)
		}
	}()

	wipGen = NewWipGenerator()

	boxId = os.Args[1]

	// TODO use ENV
	serverRoot, _ = filepath.Abs("tmp/servers")

	redisClient = createRedisClient()

	exec.Command("mkdir", "-p", serverRoot).Run()

	discoverRunningServers()

	jobChannel := make(chan Job)
	boxQueueKey := fmt.Sprintf("jobs/%s", boxId)
	go popRedisQueue(jobChannel, boxQueueKey)

	// start heartbeat
	go heartbeat()

	setStateTo("up")
	fmt.Println(
		fmt.Sprintf("[%d] queue=%s servers=%s",
			os.Getpid(),
			boxQueueKey,
			serverRoot))
	go processJobs(jobChannel)

	// trap signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGQUIT)

	// wait for signal
	<-sig
	fmt.Println("stopping pinky")
	setStateTo("down")

	// wait for no work in progress
	// TODO something better than polling wip count?
	empty := time.NewTicker(time.Second * 1)
	jobCount := -1
	for _ = range empty.C {
		if jobCount != wipGen.Count {
			fmt.Println(wipGen.Count, "jobs remaining")
		}
		jobCount = wipGen.Count

		if wipGen.Count == 0 {
			os.Exit(0)
		}
	}
}
