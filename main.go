package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/simonz05/godis/redis"
	"io/ioutil"
	"labix.org/v2/mgo/bson"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// (start|stop|broadcast|tell|multi)
type Job struct {
	Name       string
	ServerId   string
	Funpack    string
	Ram        RamAllocation
	SnapshotId string
	WorldUrl   string
	Settings   interface{}

	// for broadcast
	Msg string

	// for tell
	Username string
}

type RamAllocation struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type RedisServerInfo struct {
	Pid  int `json:"pid"`
	Port int `json:"port"`
}

type PinkyServerEvent struct {
	Ts       time.Time `json:"ts"`
	PinkyId  string    `json:"pinky_id"`
	ServerId string    `json:"server_id"`
	Type     string    `json:"type"`
	Msg      string    `json:"msg"`

	// these fields for the backed_up event
	SnapshotId string `json:"snapshot_id"`
	Url        string `json:"url"`

	// these fields for the player events
	Username  string `json:"username"`
	Usernames string `json:"url"`

	// these fields for the settings_changed events
	Actor string `json:"actor"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

var redisClient *redis.Client
var boxId string
var servers = map[string]*Server{}
var serverRoot string
var wipGen *WipGenerator
var portPool chan int
var plog *Logger // pinky logger

func popRedisQueue(c chan Job, queue string) {
	client := NewRedisConnection()
	for {
		reply, e := client.Brpop([]string{queue}, 2)
		if e != nil {
			if !strings.Contains(e.Error(), "timeout expired") {
				fmt.Println("redis error: %T", e)
			}
		} else {
			val := reply.BytesArray()[1]

			var job Job
			json.Unmarshal(val, &job)

			c <- job
		}
	}
}

func serverPath(serverId string) string {
	return filepath.Join(serverRoot, serverId)
}

func pidFile(serverId string) string {
	return filepath.Join(serverRoot, fmt.Sprintf("%s.pid", serverId))
}

func startServer(serverId string, funpack string, snapshotId string, worldUrl string, ram RamAllocation, settings interface{}) error {
	if _, present := servers[serverId]; !present {

		port := <-portPool

		server := &Server{
			Id:   serverId,
			Path: filepath.Join(serverRoot, serverId),
			Port: port,
			Log: NewLog(map[string]interface{}{
				"serverId": serverId,
			}),
		}

		servers[server.Id] = server

		plog.Info(map[string]interface{}{
			"event":    "world_starting",
			"serverId": serverId,
		})

		pidFile := pidFile(serverId)

		server.PrepareServerPath()
		if worldUrl != "" {
			err := server.DownloadWorld(worldUrl)
			if err != nil {
				return err
			}
		}
		server.DownloadFunpack(funpack)
		server.WriteSettingsFile(
			funpack,
			ram,
			settings)
		pid, err := server.StartServerProcess(pidFile)
		if err != nil {
			return err
		}

		events := server.Monitor()
		go processServerEvents(serverId, events, false)

		serverInfo := RedisServerInfo{
			Pid:  pid,
			Port: port,
		}
		serverJson, err := json.Marshal(serverInfo)
		if err != nil {
			return err
		}
		redisClient.Set(
			fmt.Sprintf("pinky:%s:servers:%s", boxId, serverId),
			serverJson)

	} else {
		plog.Info(map[string]interface{}{
			"event":  "job_ignored",
			"reason": "already started",
			"server": serverId,
		})
	}
	return nil
}

func runBackups(stop chan bool, serverId string) {
	ticker := time.NewTicker(10 * time.Minute)

	for {
		select {
		case <-ticker.C:
			wip := <-wipGen.C
			plog.Info(map[string]interface{}{
				"event":    "periodic_backup_starting",
				"serverId": serverId,
			})

			err := backupServer(serverId, time.Now())
			if err != nil {
				// TODO figure out some recovery scenario
				plog.Error(err, map[string]interface{}{
					"event": "backup_failed",
				})
			}
			plog.Info(map[string]interface{}{
				"event":    "periodic_backup_completed",
				"serverId": serverId,
			})

			wip <- true

		case <-stop:
			ticker.Stop()
			return
		}
	}
}

func startTicking(serverId string, stop chan bool) {
	minute := time.NewTicker(60 * time.Second)
	go func() {
		for {
			select {
			case <-minute.C:
				pushServerEvent(PinkyServerEvent{
					PinkyId:  boxId,
					ServerId: serverId,
					Ts:       time.Now(),
					Type:     "minute",
				})
			case <-stop:
				minute.Stop()
			}

		}
	}()
}

func processServerEvents(serverId string, events chan ServerEvent, attached bool) {
	stopBackups := make(chan bool, 1)
	stopTicks := make(chan bool, 1)

	hasWorld := servers[serverId].HasWorld
	if hasWorld {
		go runBackups(stopBackups, serverId)
	}

	if attached {
		// we are reattaching to a running server
		// so we can't wait for a started event
		// we need to start ticking straight away
		startTicking(serverId, stopTicks)
	}

	var wip chan bool
	for event := range events {
		plog.Info(map[string]interface{}{
			"ts":          event.Ts,
			"event":       "server_event",
			"serverId":    serverId,
			"serverEvent": event.Event,
			"serverMsg":   event.Msg,
		})

		switch event.Event {
		case "started":
			transitionStartingToUp(serverId)
			startTicking(serverId, stopTicks)
		case "stopping":
			if attemptStoppingTransition(serverId) {
				wip = <-wipGen.C
			}
		case "fatal_error":
			servers[serverId].Kill()
		}

		pushServerEvent(PinkyServerEvent{
			PinkyId:   boxId,
			ServerId:  serverId,
			Ts:        event.Ts,
			Type:      event.Event,
			Msg:       event.Msg,
			Username:  event.Username,
			Usernames: event.Usernames,
			Actor:     event.Actor,
			Key:       event.Key,
			Value:     event.Value,
		})
	}

	plog.Info(map[string]interface{}{
		"event":    "server_events_exit",
		"serverId": serverId,
	})

	if wip == nil {
		wip = <-wipGen.C
	}
	defer close(wip)

	stopTicks <- true
	stopBackups <- true

	state, _ := redisClient.Get(stateKey(serverId))

	if hasWorld && string(state) != "starting" {
		plog.Info(map[string]interface{}{
			"event":    "shutdown_backup_starting",
			"serverId": serverId,
		})
		err := backupServer(serverId, time.Now())
		if err != nil {
			// TODO figure out some recovery scenario
			plog.Error(errors.New("Shutdown backup failed"), map[string]interface{}{
				"event":    "shutdown_backup_failed",
				"serverId": serverId,
			})
		}
		plog.Info(map[string]interface{}{
			"event":    "shutdown_backup_completed",
			"serverId": serverId,
		})
	}

	transitionToStopped(serverId)
	removeServerArtifacts(serverId)
	portPool <- servers[serverId].Port
	delete(servers, serverId)

	pushServerEvent(PinkyServerEvent{
		PinkyId:  boxId,
		ServerId: serverId,
		Ts:       time.Now(),
		Type:     "stopped",
	})

	plog.Info(map[string]interface{}{
		"event":    "server_stopped",
		"serverId": serverId,
	})
}

func backupServer(serverId string, backupTime time.Time) (err error) {
	wip := <-wipGen.C
	defer close(wip)

	server := servers[serverId]
	if server == nil {
		err = errors.New(fmt.Sprintf("no server for %s", serverId))
		return
	}

	var url string
	err = retry(10, 5*time.Second, func() error {
		var err error
		url, err = server.BackupWorld(backupTime)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":    "world_backup",
				"serverId": serverId,
			})
		}
		return err
	})
	if err != nil {
		return
	}

	var snapshotId bson.ObjectId
	err = retry(10, 5*time.Second, func() error {
		var err error
		snapshotId, err = storeBackupInMongo(serverId, url, backupTime)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":    "world_db_store",
				"serverId": serverId,
			})

		}
		return err
	})

	pushServerEvent(PinkyServerEvent{
		Ts:         backupTime,
		PinkyId:    boxId,
		ServerId:   serverId,
		Type:       "backed_up",
		SnapshotId: snapshotId.Hex(),
		Url:        url,
	})

	return
}

func stopServer(serverId string) {
	if attemptStoppingTransition(serverId) {
		wip := <-wipGen.C
		defer close(wip)

		server := servers[serverId]
		if server == nil {
			plog.Error(errors.New("Server not found"), map[string]interface{}{
				"event":    "stop_server",
				"serverId": serverId,
			})
			return
		}
		server.Stop()
	} else {
		plog.Info(map[string]interface{}{
			"event":    "stop request ignored",
			"serverId": serverId,
		})
	}
}

func broadcast(serverId string, message string) {
	server := servers[serverId]
	if server == nil {
		plog.Error(errors.New("Server not found"), map[string]interface{}{
			"event":    "broadcast",
			"serverId": serverId,
		})
		return
	}
	server.Broadcast(message)
}

func tell(serverId string, username string, message string) {
	server := servers[serverId]
	if server == nil {
		plog.Error(errors.New("Server not found"), map[string]interface{}{
			"event":    "tell",
			"serverId": serverId,
		})
		return
	}
	server.Tell(username, message)
}

func listPlayers(serverId string) {
	server := servers[serverId]
	if server == nil {
		plog.Error(errors.New("Server not found"), map[string]interface{}{
			"event":    "list_players",
			"serverId": serverId,
		})
		return
	}
	server.ListPlayers()
}

func removeServerArtifacts(serverId string) {
	exec.Command("rm", "-f", pidFile(serverId)).Run()
	exec.Command("rm", "-rf", serverPath(serverId)).Run()

	redisClient.Del(
		fmt.Sprintf("pinky:%s:servers:%s", boxId, serverId))
	redisClient.Del(
		fmt.Sprintf("server:%s:state", serverId))
	redisClient.Del(
		fmt.Sprintf("server:%s:slots", serverId))
	redisClient.Del(
		fmt.Sprintf("server:%s:players", serverId))
}

func handleRedisError(err error) {
	if err != nil {
		plog.Error(err, map[string]interface{}{
			"event": "redis_connection_error",
		})
	}
}

func pushServerEvent(event PinkyServerEvent) {
	pseJson, _ := json.Marshal(event)
	redisClient.Lpush("server:events", pseJson)
}

func stateKey(serverId string) string {
	return fmt.Sprintf("server:%s:state", serverId)
}

func pinkyServerKey(serverId string) string {
	return fmt.Sprintf("pinky:%s:servers:%s", boxId, serverId)
}

func pinkyServersKey() string {
	return fmt.Sprintf("pinky:%s:servers:*", boxId)
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
	err := retry(50, 100*time.Millisecond, func() error {
		var err error
		result, err = redisClient.Get(key)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event": "redis_get_failed",
				"key":   key,
			})
		}
		return err
	})
	handleRedisError(err)
	return result
}

func redisDel(key string) int64 {
	var result int64
	err := retry(50, 100*time.Millisecond, func() error {
		var err error
		result, err = redisClient.Del(key)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event": "redis_del_failed",
				"key":   key,
			})
		}
		return err
	})
	handleRedisError(err)
	return result
}

func redisSet(key string, value []byte) {
	err := retry(50, 100*time.Millisecond, func() error {
		var err error
		err = redisClient.Set(key, value)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event": "redis_set_failed",
				"key":   key,
			})
		}
		return err
	})
	handleRedisError(err)
	return
}

func stateTransition(
	serverId string,
	from string,
	to string,
	enforceStartingCondition bool) bool {

	// TODO race condition?
	serverStateKey := stateKey(serverId)
	oldValue := string(redisGet(serverStateKey))

	if oldValue != from {
		msg := fmt.Sprintf(
			"invalid state! Expected %s was %s (%s)",
			from, oldValue, serverStateKey)
		if enforceStartingCondition {
			plog.Error(errors.New(msg), map[string]interface{}{
				"event": "state_transition",
				"from":  from,
				"to":    to,
			})
			return false

		} else {
			return false
		}
	}

	if to != "" {
		redisSet(serverStateKey, []byte(to))
	} else {
		redisDel(serverStateKey)
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
	key := fmt.Sprintf("pinky:%s:state", boxId)
	return redisGet(key)
}

func setStateTo(state string) {
	key := fmt.Sprintf("pinky:%s:state", boxId)
	redisClient.Set(key, []byte(state))
}

func processJobs(jobChannel chan Job) {
	for {
		job := <-jobChannel

		if string(getState()) == "down" {
			plog.Info(map[string]interface{}{
				"event":  "job_ignored",
				"reason": "pinky is down",
				"job":    job,
			})
			continue
		}

		switch job.Name {

		case "start":
			go func() {
				wip := <-wipGen.C
				startServer(
					job.ServerId,
					job.Funpack,
					job.SnapshotId,
					job.WorldUrl,
					job.Ram,
					job.Settings)
				close(wip)
			}()

		case "stop":
			go stopServer(job.ServerId)

		case "broadcast":
			go broadcast(job.ServerId, job.Msg)

		case "tell":
			go tell(job.ServerId, job.Username, job.Msg)

		case "list":
			go listPlayers(job.ServerId)

		default:
			plog.Info(map[string]interface{}{
				"event":  "job_ignored",
				"reason": "job unknown",
				"job":    job,
			})
		}
	}
}

func isAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// returns serverId, pid
func readPidFromFile(pidFile string) (serverId string, pid int, err error) {
	b, err := ioutil.ReadFile(pidFile)
	if err != nil {
		return
	}
	pid, err = strconv.Atoi(string(b))
	if err != nil {
		return
	}

	parts := strings.Split(filepath.Base(pidFile), ".")
	serverId = parts[0]

	return
}

func discoverRunningServers() {
	matches, err := filepath.Glob(filepath.Join(serverRoot, "*.pid"))
	if err != nil {
		panic(err)
	}
	for _, pidFile := range matches {
		serverId, pid, err := readPidFromFile(pidFile)
		if err != nil {
			panic(err)
		}
		serverPath := serverPath(serverId)
		plog.Info(map[string]interface{}{
			"event":      "server_pid_found",
			"pidFile":    pidFile,
			"serverPath": serverPath,
		})

		if isAlive(pid) {
			plog.Info(map[string]interface{}{
				"event":    "running_server",
				"serverId": serverId,
				"pid":      pid,
			})

			var serverInfo RedisServerInfo
			jsonVal := redisGet(pinkyServerKey(serverId))
			err = json.Unmarshal(jsonVal, &serverInfo)
			if err != nil {
				plog.Error(err, map[string]interface{}{
					"event":    "read_server_redis",
					"rawValue": jsonVal,
				})

				// TODO recover
				panic(err)
			}

			servers[serverId] = AttachServer(
				serverId,
				serverPath,
				pid,
				serverInfo.Port)

			events := servers[serverId].Monitor()
			go processServerEvents(serverId, events, true)

		} else {
			plog.Info(map[string]interface{}{
				"event": "dead_server_found",
				"pid":   pid,
			})

			removeServerArtifacts(serverId)
		}
	}
}

func cleanOldServers() error {
	keys, err := redisClient.Keys(pinkyServersKey())
	if err != nil {
		return err
	}
	for _, key := range keys {
		serverId := strings.Split(key, ":")[3]
		if _, ok := servers[serverId]; !ok {
			removeServerArtifacts(serverId)
			plog.Info(map[string]interface{}{
				"event":    "dead_server_removed",
				"serverId": serverId,
			})

		}
	}
	return nil
}

func portsInUse() []int {
	ports := make([]int, 100)
	for _, server := range servers {
		ports = append(ports, server.Port)
	}
	return ports
}

func waitForNoWorkInProgress() {
	empty := time.NewTicker(time.Second * 1)
	jobCount := -1
	for _ = range empty.C {
		if jobCount != wipGen.Count {
			plog.Info(map[string]interface{}{
				"event": "waiting_wip_remaining",
				"count": wipGen.Count,
			})
		}
		jobCount = wipGen.Count

		if wipGen.Count == 0 {
			return
		}
	}
}

func main() {
	boxId = os.Args[1]

	plog = NewLog(map[string]interface{}{"boxId": boxId})
	if r := recover(); r != nil {
		// TODO bugsnag
		fmt.Println("ERMAHGERD FERTEL ERRERRRR! ಠ_ಠ")
		panic(r)
	}

	wipGen = NewWipGenerator()

	serverRoot = os.Getenv("SERVERS_PATH")
	if serverRoot == "" {
		serverRoot, _ = filepath.Abs("tmp/servers")
	}

	redisClient = NewRedisConnection()

	exec.Command("mkdir", "-p", serverRoot).Run()

	discoverRunningServers()
	err := cleanOldServers()
	if err != nil {
		panic(err)
	}

	// allocate 200 ports starting at 10000 with a 100 port gap
	portPool = NewIntPool(10000, 200, 100, portsInUse())

	jobChannel := make(chan Job)
	boxQueueKey := fmt.Sprintf("pinky:%s:in", boxId)
	go popRedisQueue(jobChannel, boxQueueKey)

	// start heartbeat
	go heartbeat(serverRoot)

	setStateTo("up")

	plog.Out("info", map[string]interface{}{
		"event":   "pinky_up",
		"queue":   boxQueueKey,
		"servers": serverRoot,
	})

	go processJobs(jobChannel)

	// trap signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGQUIT)

	// wait for signal
	signal := <-sig
	plog.Info(map[string]interface{}{
		"event":  "pinky_stopping",
		"signal": signal,
	})

	setStateTo("down")

	waitForNoWorkInProgress()
}
