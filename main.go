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

	// Chat
	Msg  string `json:"msg"`
	Nick string `json:"nick"`

	// these fields for the backed_up event
	SnapshotId string `json:"snapshot_id"`
	Url        string `json:"url"`
	Size       int64  `json:"size"`

	// these fields for the player events
	Auth string   `json:"auth"`
	Uids []string `json:"uids"`
	Uid  string   `json:"uid"`

	// Deprecated
	Username  string   `json:"username"`
	Usernames []string `json:"usernames"`

	// these fields for the settings_changed events
	Actor string `json:"actor"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

var redisClient *redis.Client
var boxId string
var servers = NewServers()
var serverRoot string
var wipGen *WipGenerator
var portPool chan int
var plog DataLogger // pinky logger

func serverPath(serverId string) string {
	return filepath.Join(serverRoot, serverId)
}

func pidFile(serverId string) string {
	return filepath.Join(serverRoot, fmt.Sprintf("%s.pid", serverId))
}

func startServer(serverId string, funpackId string, funpackUrl string, snapshotId string, worldUrl string, ram RamAllocation, settings interface{}) error {
	if !servers.Exists(serverId) {

		port := <-portPool

		server := servers.Add(&Server{
			Id:   serverId,
			Path: filepath.Join(serverRoot, serverId),
			Port: port,
			Log: NewLog(map[string]interface{}{
				"serverId": serverId,
			}),
			State: "starting",
		})

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
		err := server.DownloadFunpack(funpackId, funpackUrl, worldUrl != "")
		if err != nil {
			return err
		}

		server.WriteSettingsFile(
			funpackId,
			funpackUrl,
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

func processServerEvents(serverId string, events chan ServerEvent, attached bool) {
	stopBackups := make(chan bool, 1)
	stopTicks := make(chan bool, 1)

	// Not sure why this is happening:
	if !servers.Exists(serverId) {
		return
	}

	hasWorld := servers.Get(serverId).HasWorld

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

	var stopWip chan bool

	server := servers.Get(serverId)

	for event := range events {
		switch event.Event {
		case "started":
			server.State = "up"
		case "stopping":
			if server.State != "stopping" {
				server.State = "stopping"
				stopWip = <-wipGen.C
			}

		case "fatal_error":
			server.Kill(syscall.SIGKILL)
			go server.EnsureServerStopped()
		}

		pushServerEvent(PinkyServerEvent{
			PinkyId:   boxId,
			ServerId:  serverId,
			Ts:        event.Ts,
			Type:      event.Event,
			Nick:      event.Nick,
			Msg:       event.Msg,
			Auth:      event.Auth,
			Uid:       event.Uid,
			Uids:      event.Uids,
			Usernames: event.Usernames,
			Username:  event.Username,
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

	if hasWorld && servers.Get(serverId).State != "starting" {
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

	// Not sure why this is happening:
	if server := servers.Get(serverId); server != nil {
		portPool <- server.Port
		servers.Del(serverId)
	}

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

	server := servers.Get(serverId)
	if server == nil {
		err = errors.New(fmt.Sprintf("no server for %s", serverId))
		return
	}

	var (
		url  string
		size int64
	)
	err = retry(1000, 5*time.Second, func(retries int) error {
		var err error
		url, size, err = server.BackupWorld(backupTime)
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
	err = retry(1000, 5*time.Second, func(retries int) error {
		var err error
		snapshotId, err = StoreBackupInMongo(serverId, url, size, backupTime)
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
		Size:       size,
	})

	return
}

func stopServer(serverId string) {
	if server := servers.Get(serverId); server != nil {
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

func ServerOp(serverId string, name string, op func(*Server)) {
	if server := servers.Get(serverId); server != nil {
		op(server)
	} else {
		plog.Error(errors.New("Server not found"), map[string]interface{}{
			"event":    name,
			"serverId": serverId,
		})
	}
}

func broadcast(serverId string, message string) {
	ServerOp(serverId, "broadcast", func(server *Server) {
		server.Broadcast(message)
	})
}

func tell(serverId string, username string, message string) {
	ServerOp(serverId, "tell", func(server *Server) {
		server.Tell(username, message)
	})
}

func listPlayers(serverId string) {
	ServerOp(serverId, "tell", func(server *Server) {
		server.ListPlayers()
	})
}

func kickPlayer(serverId string, username string, message string) {
	ServerOp(serverId, "tell", func(server *Server) {
		server.Kick(username, message)
	})
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
		"nick":        event.Nick,
		"msg":         event.Msg,
		"snapshotId":  event.SnapshotId,
		"url":         event.Url,
		"size":        event.Size,
		"auth":        event.Auth,
		"uid":         event.Uid,
		"uids":        event.Uids,
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
					job.FunpackId,
					job.FunpackUrl,
					job.SnapshotId,
					job.WorldUrl,
					job.Ram,
					job.Settings)

				if err != nil {
					servers.Del(job.ServerId)
					plog.Error(err, map[string]interface{}{
						"event":  "server_start_error",
						"server": job.ServerId,
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

		if IsRunning(pid) {
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
				// panic(err)
				continue
			}

			server, err := AttachServer(
				serverId,
				serverPath,
				pid,
				serverInfo.Port)
			if err != nil {
				panic(err)
				continue
			}

			events, err := server.Monitor()
			if err != nil {
				panic(err)
				continue
			}
			servers.Add(server)
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
		if !servers.Exists(serverId) {
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

	// test redis connection
	redisClient = NewRedisConnection()

	// test mongo connection
	serverCount, err := CountServers()
	if err != nil {
		panic(err)
	}

	exec.Command("mkdir", "-p", serverRoot).Run()

	discoverRunningServers()

	err = cleanOldServers()
	if err != nil {
		panic(err)
	}

	// allocate 200 ports starting at 10000 with a 100 port gap
	portPool = NewIntPool(10000, 200, 100, servers.PortsReserved())

	boxQueueKey := fmt.Sprintf("pinky:%s:in", boxId)
	jobPopper := NewJobPopper(boxQueueKey)

	// start heartbeat
	go heartbeat(serverRoot)
	go servers.Tick()

	setStateTo("up")

	var fdLimit syscall.Rlimit
	syscall.Getrlimit(syscall.RLIMIT_NOFILE, &fdLimit)

	plog.Out("info", map[string]interface{}{
		"event":        "pinky_up",
		"queue":        boxQueueKey,
		"servers":      serverRoot,
		"fd_current":   fdLimit.Cur,
		"fd_max":       fdLimit.Max,
		"server_count": serverCount,
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
