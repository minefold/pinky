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
	Username  string   `json:"username"`
	Usernames []string `json:"usernames"`

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
			State: "starting",
		}

		servers[server.Id] = server

		plog.Info(map[string]interface{}{
			"event":    "server_starting",
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
		server.Compile("/opt/funpacks/steam", "/opt/funpacks/cache")
		server.WriteSettingsFile(
			funpack,
			ram,
			settings)

		pid, err := server.StartServerProcess(pidFile)
		if err != nil {
			return err
		}

		events, err := server.Monitor()
		if err != nil {
			return err
		}

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
		pushServerEvent(PinkyServerEvent{
			PinkyId:  boxId,
			ServerId: serverId,
			Ts:       time.Now(),
			Type:     "started",
		})
	}
	return nil
}

func runBackups(stop chan bool, serverId string) {
	ticker := time.NewTicker(10 * time.Minute)

	for {
		select {
		case <-ticker.C:
			runPeriodicBackup(serverId)

		case <-stop:
			ticker.Stop()
			plog.Info(map[string]interface{}{
				"event":    "periodic_backups_stopped",
				"serverId": serverId,
			})
			return
		}
	}
}

func runPeriodicBackup(serverId string) {
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
		plog.Info(map[string]interface{}{
			"event":    "starting_periodic_backups",
			"serverId": serverId,
		})
	} else {
		plog.Info(map[string]interface{}{
			"event":    "no_periodic_backups",
			"serverId": serverId,
		})
	}

	if attached {
		// we are reattaching to a running server
		// so we can't wait for a started event
		// we need to start ticking straight away
		startTicking(serverId, stopTicks)
	}

	var stopWip chan bool

	for event := range events {
		switch event.Event {
		case "started":
			servers[serverId].State = "up"
			startTicking(serverId, stopTicks)
		case "stopping":
			if servers[serverId].State != "stopping" {
				servers[serverId].State = "stopping"
				stopWip = <-wipGen.C
			}

		case "fatal_error":
			servers[serverId].Kill(syscall.SIGKILL)
			go servers[serverId].EnsureServerStopped()
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

	if stopWip == nil {
		stopWip = <-wipGen.C
	}
	defer close(stopWip)

	stopTicks <- true
	stopBackups <- true

	if hasWorld && servers[serverId].State != "starting" {
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
	err = retry(10, 5*time.Second, func(retries int) error {
		var err error
		url, err = server.BackupWorld(backupTime)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":    "world_backup",
				"serverId": serverId,
				"retries":  retries,
			})
		}
		return err
	})
	if err != nil {
		return
	}

	var snapshotId bson.ObjectId
	err = retry(10, 5*time.Second, func(retries int) error {
		var err error
		snapshotId, err = storeBackupInMongo(serverId, url, backupTime)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":    "world_db_store",
				"serverId": serverId,
				"retries":  retries,
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
	server, ok := servers[serverId]
	if ok {
		wip := <-wipGen.C
		defer close(wip)

		server.Stop()
	} else {
		plog.Info(map[string]interface{}{
			"event":    "stop_request_ignored",
			"reason":   "server not found",
			"serverId": serverId,
		})
		pushServerEvent(PinkyServerEvent{
			PinkyId:  boxId,
			ServerId: serverId,
			Ts:       time.Now(),
			Type:     "stopped",
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

func kickPlayer(serverId string, username string, message string) {
	server := servers[serverId]
	if server == nil {
		plog.Error(errors.New("Server not found"), map[string]interface{}{
			"event":    "kick",
			"serverId": serverId,
		})
		return
	}
	server.Kick(username, message)
}

func removeServerArtifacts(serverId string) {
	exec.Command("rm", "-f", pidFile(serverId)).Run()
	exec.Command("rm", "-rf", serverPath(serverId)).Run()

	redisClient.Del(
		fmt.Sprintf("pinky:%s:servers:%s", boxId, serverId))
}

func handleRedisError(err error) {
	if err != nil {
		plog.Error(err, map[string]interface{}{
			"event": "redis_connection_error",
		})
	}
}

func pushServerEvent(event PinkyServerEvent) {
	plog.Info(map[string]interface{}{
		"ts":          event.Ts,
		"event":       "server_event",
		"serverId":    event.ServerId,
		"serverEvent": event.Type,
		"msg":         event.Msg,
		"snapshotId":  event.SnapshotId,
		"url":         event.Url,
		"username":    event.Username,
		"usernames":   event.Usernames,
		"actor":       event.Actor,
		"key":         event.Key,
		"value":       event.Value,
	})

	pseJson, _ := json.Marshal(event)
	redisClient.Lpush("server:events", pseJson)
}

func pinkyServerKey(serverId string) string {
	return fmt.Sprintf("pinky:%s:servers:%s", boxId, serverId)
}

func pinkyServersKey() string {
	return fmt.Sprintf("pinky:%s:servers:*", boxId)
}

func retry(maxRetries int, delay time.Duration, work func(retries int) error) error {
	retries := 0
	err := work(retries)
	for err != nil && retries < maxRetries {
		time.Sleep(delay)
		err = work(retries)
		retries += 1
	}
	return err
}

func redisGet(key string) []byte {
	var result []byte
	err := retry(50, 100*time.Millisecond, func(retries int) error {
		var err error
		result, err = redisClient.Get(key)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":   "redis_get_failed",
				"key":     key,
				"retries": retries,
			})
		}
		return err
	})
	handleRedisError(err)
	return result
}

func redisDel(key string) int64 {
	var result int64
	err := retry(50, 100*time.Millisecond, func(retries int) error {
		var err error
		result, err = redisClient.Del(key)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":   "redis_del_failed",
				"key":     key,
				"retries": retries,
			})
		}
		return err
	})
	handleRedisError(err)
	return result
}

func redisSet(key string, value []byte) {
	err := retry(50, 100*time.Millisecond, func(retries int) error {
		var err error
		err = redisClient.Set(key, value)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":   "redis_set_failed",
				"key":     key,
				"retries": retries,
			})
		}
		return err
	})
	handleRedisError(err)
	return
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
	for job := range jobChannel {
		pinkyState := string(getState())
		if pinkyState == "down" {
			plog.Info(map[string]interface{}{
				"event":  "job_ignored",
				"reason": "pinky is down",
				"job":    job,
			})
			continue
		}

		if pinkyState == "stopping" && job.Name == "start" {
			plog.Info(map[string]interface{}{
				"event":  "start_job_ignored",
				"reason": "pinky is stopping",
				"job":    job,
			})
			pushServerEvent(PinkyServerEvent{
				PinkyId:  boxId,
				ServerId: job.ServerId,
				Ts:       time.Now(),
				Type:     "stopped",
			})
			continue
		}

		plog.Info(map[string]interface{}{
			"event":  "job_received",
			"name":   job.Name,
			"server": job.ServerId,
		})

		switch job.Name {

		case "start":
			go func() {
				wip := <-wipGen.C
				defer close(wip)
				err := startServer(
					job.ServerId,
					job.Funpack,
					job.SnapshotId,
					job.WorldUrl,
					job.Ram,
					job.Settings)

				if err != nil {
					plog.Error(err, map[string]interface{}{
						"event": "server_start_error",
					})
					pushServerEvent(PinkyServerEvent{
						PinkyId:  boxId,
						ServerId: job.ServerId,
						Ts:       time.Now(),
						Type:     "stopped",
					})
				}

			}()

		case "stop":
			go stopServer(job.ServerId)

		case "broadcast":
			go broadcast(job.ServerId, job.Msg)

		case "tell":
			go tell(job.ServerId, job.Username, job.Msg)

		case "list":
			go listPlayers(job.ServerId)

		case "kick":
			go kickPlayer(job.ServerId, job.Username, job.Msg)

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

			events, err := servers[serverId].Monitor()
			if err != nil {
				return
			}
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
			pushServerEvent(PinkyServerEvent{
				PinkyId:  boxId,
				ServerId: serverId,
				Ts:       time.Now(),
				Type:     "stopped",
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

	boxQueueKey := fmt.Sprintf("pinky:%s:in", boxId)
	jobPopper := NewJobPopper(boxQueueKey)

	// start heartbeat
	go heartbeat(serverRoot)

	setStateTo("up")

	var fdLimit syscall.Rlimit
	syscall.Getrlimit(syscall.RLIMIT_NOFILE, &fdLimit)

	plog.Out("info", map[string]interface{}{
		"event":      "pinky_up",
		"queue":      boxQueueKey,
		"servers":    serverRoot,
		"fd_current": fdLimit.Cur,
		"fd_max":     fdLimit.Max,
	})

	go processJobs(jobPopper.C)

	// trap signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGQUIT)

	// wait for signal
	signal := <-sig
	plog.Info(map[string]interface{}{
		"event":  "pinky_stopping",
		"signal": signal,
	})

	// stop popping jobs
	jobPopper.Stop()

	waitForNoWorkInProgress()
}
