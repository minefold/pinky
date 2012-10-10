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
	"runtime/debug"
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
var boxId string
var servers = map[string]*Server{}
var workInProgress = map[string]string{}

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

		events := server.Monitor()
		go processServerEvents(job.ServerId, pidFile, events)
	} else {
		fmt.Println("Ignoring start request")
	}
}

func processServerEvents(serverId string, pidFile string, events chan ServerEvent) {
	for event := range events {
		fmt.Println(event)
		switch event.Event {
		case "started":
			transitionStartingToUp(serverId)
		case "stopping":
			attemptStoppingTransition(serverId)
		}
	}
	transitionStoppingToStopped(serverId)
	fmt.Println("server stopped")
	exec.Command("rm", "-f", pidFile).Run()
	delete(servers, serverId)
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
	var value []byte
	err := retry(5, 100*time.Millisecond, func() error {
		var err error
		value, err = redisClient.Get(key)
		return err
	})
	handleRedisError(err)
	return value
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

func transitionStoppingToStopped(serverId string) {
	stateTransition(serverId, "stopping", "", true)
}

func getState() []byte {
	key := fmt.Sprintf("pinky/%s/state", boxId)
	return redisGet(key)
}

func setStateTo(state string) {
	key := fmt.Sprintf("pinky/%s/state", boxId)
	redisClient.Set(key, []byte(state))
}

func uuid() string {
	id, _ := exec.Command("uuidgen").Output()
	return strings.TrimSpace(string(id))
}

func doWork(empty chan bool, name string, work func()) {
	id := uuid()
	workInProgress[id] = id
	work()
	delete(workInProgress, id)
	empty <- len(workInProgress) == 0
}

func processJobs(empty chan bool, jobChannel chan Job, serverRoot string) {
	for {
		job := <-jobChannel
		if string(getState()) == "down" {
			fmt.Println("ignoring job: pinky is down")
			continue
		}

		switch job.Name {

		case "start":
			go doWork(empty, "start", func() {
				startServer(job, serverRoot)
			})

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

			events := server.Monitor()
			go processServerEvents(serverId, pidFile, events)

		} else {
			fmt.Println("found dead process", pid)
		}
	}
}

type hbJson struct {
	Disk  map[string]DiskUsage `json:"disk"`
	Procs map[string]ProcUsage `json:"procs"`
}

func heartbeatJson() []byte {
	procUsages, _ := CollectProcUsage()
	diskUsages, _ := CollectDiskUsage()

	v := hbJson{
		Disk:  make(map[string]DiskUsage),
		Procs: make(map[string]ProcUsage),
	}

	for _, diskUsage := range diskUsages {
		v.Disk[diskUsage.Name] = diskUsage
	}
	for _, procUsage := range procUsages {
		v.Procs[fmt.Sprintf("%d", procUsage.Pid)] = procUsage
	}

	json, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	return json
}

func quitWhenEmpty(empty chan bool) {
	for finished := range empty {
		if finished {
			os.Exit(0)
		} else {
			fmt.Println(
				fmt.Sprintf("Quitting. %d jobs remaining",
					len(workInProgress)))
		}
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			// TODO bugsnag
			fmt.Println("ERMAHGERD FERTEL ERRERRRR!")
			panic(r)
		}
	}()

	boxId = os.Args[1]

	redisClient = createRedisClient()

	serverRoot, _ := filepath.Abs("tmp/servers")
	exec.Command("mkdir", "-p", serverRoot).Run()

	discoverRunningServers(serverRoot)

	jobChannel := make(chan Job)
	boxQueueKey := fmt.Sprintf("jobs/%s", boxId)
	go popRedisQueue(jobChannel, boxQueueKey)

	// start heartbeat
	ticker := time.NewTicker(time.Second * 10)
	go func() {
		for _ = range ticker.C {
			json := heartbeatJson()
			key := fmt.Sprintf("pinky/%s/resources", boxId)

			redisClient.Setex(key, 20, json)
		}
	}()

	// trap signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGQUIT)

	empty := make(chan bool, 100)

	go func() {
		for {
			<-sig
			setStateTo("down")
			if len(workInProgress) == 0 {
				os.Exit(0)
			}
			quitWhenEmpty(empty)
		}
	}()

	setStateTo("up")
	fmt.Println(
		fmt.Sprintf("[%d] processing queue: %s", os.Getpid(), boxQueueKey))
	processJobs(empty, jobChannel, serverRoot)
}
