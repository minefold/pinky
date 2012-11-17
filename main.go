package main

import (
	"encoding/json"
	"fmt"
	// "log"
	"github.com/simonz05/godis/redis"
	"io/ioutil"
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

type PinkyServerEvent struct {
	Ts       time.Time `json:"ts"`
	PinkyId  string    `json:"pinky_id"`
	ServerId string    `json:"server_id"`
	Type     string    `json:"type"`
	Msg      string    `json:"msg"`
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

func startServer(job Job) error {
	// if attemptStartingTransition(job.ServerId) {
	if _, present := servers[job.ServerId]; !present {

		port := <-portPool

		server := &Server{
			Id:   job.ServerId,
			Path: filepath.Join(serverRoot, job.ServerId),
			Port: port,
			Log: NewLog(map[string]interface{}{
				"serverId": job.ServerId,
			}),
		}

		servers[server.Id] = server

		plog.Info(map[string]interface{}{
			"event":    "world_starting",
			"serverId": job.ServerId,
		})

		pidFile := pidFile(job.ServerId)

		server.PrepareServerPath()
		if job.World != "" {
			err := server.DownloadWorld(job.World)
			if err != nil {
				return err
			}
		}
		server.DownloadFunpack(job.Funpack)
		server.WriteSettingsFile(
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
			fmt.Sprintf("pinky:%s:servers:%s", boxId, job.ServerId),
			serverJson)

	} else {
		plog.Info(map[string]interface{}{
			"event":  "job_ignored",
			"reason": "already started",
			"job":    job,
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

			err := backupServer(serverId)
			if err != nil {
				// TODO figure out some recovery scenario
				panic(err)
			}
			plog.Info(map[string]interface{}{
				"event":    "periodic_backup_completed",
				"serverId": serverId,
			})
			wip <- true

		case <-stop:
			ticker.Stop()
			return

		default:
			// TODO check if this is actually needed
			// I think this sleep might stop the cpu
			// from getting hammered
			time.Sleep(1 * time.Second)
		}
	}
}

func processServerEvents(serverId string, events chan ServerEvent) {
	stopBackups := make(chan bool)

	hasWorld := servers[serverId].HasWorld
	if hasWorld {
		go runBackups(stopBackups, serverId)
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
		case "stopping":
			if attemptStoppingTransition(serverId) {
				wip = <-wipGen.C
			}
		}

		pse := PinkyServerEvent{
			PinkyId:  boxId,
			ServerId: serverId,
			Ts:       event.Ts,
			Type:     event.Event,
			Msg:      event.Msg,
		}
		pseJson, _ := json.Marshal(pse)

		redisClient.Lpush(fmt.Sprintf("server:events"), pseJson)
	}

	if wip == nil {
		wip = <-wipGen.C
	}
	defer close(wip)

	if hasWorld {
		stopBackups <- true

		plog.Info(map[string]interface{}{
			"event":    "shutdown_backup_starting",
			"serverId": serverId,
		})
		err := backupServer(serverId)
		if err != nil {
			// TODO figure out some recovery scenario
			panic(err)
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
	plog.Info(map[string]interface{}{
		"event":    "server_stopped",
		"serverId": serverId,
	})
}

func backupServer(serverId string) error {
	wip := <-wipGen.C
	defer close(wip)

	server := servers[serverId]
	if server == nil {
		panic(fmt.Sprintf("no server for %s", serverId))
	}

	// TODO check if we need to do a backup
	// eg. the world didn't start properly or
	// this game has no persistent state
	backupTime := time.Now()
	var key string
	err := retry(10, 5*time.Second, func() error {
		var err error
		key, err = server.BackupWorld(backupTime)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":    "world_backup",
				"serverId": serverId,
			})
		}
		return err
	})
	if err != nil {
		return err
	}

	err = retry(10, 5*time.Second, func() error {
		err := storeBackupInMongo(serverId, key, backupTime)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event":    "world_db_store",
				"serverId": serverId,
			})

		}
		return err
	})

	return err
}

func stopServer(job Job) {
	if attemptStoppingTransition(job.ServerId) {
		wip := <-wipGen.C
		defer close(wip)

		server := servers[job.ServerId]
		if server == nil {
			panic(fmt.Sprintf("no server for %s", job.ServerId))
		}
		server.Stop()
	} else {
		plog.Info(map[string]interface{}{
			"event":    "stop request ignored",
			"serverId": job.ServerId,
		})
	}
}

func removeServerArtifacts(serverId string) {
	exec.Command("rm", "-f", pidFile(serverId)).Run()
	exec.Command("rm", "-rf", serverPath(serverId)).Run()

	redisClient.Del(
		fmt.Sprintf("pinky:%s:servers:%s", boxId, serverId))
	redisClient.Del(
		fmt.Sprintf("server:%s:state", serverId))
}

func handleRedisError(err error) {
	// TODO something more sane
	if err != nil {
		panic(err)
	}
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
			panic(msg)
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
				startServer(job)
				close(wip)
			}()

		case "stop":
			go stopServer(job)

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

			key := pinkyServerKey(serverId)
			var serverInfo RedisServerInfo
			jsonVal := redisGet(key)
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
			go processServerEvents(serverId, events)

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
