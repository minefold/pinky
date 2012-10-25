package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type hbJson struct {
	Disk  map[string]DiskUsage `json:"disk"`
	Procs map[string]ProcUsage `json:"procs"`
}

func heartbeat() {
	ticker := time.NewTicker(time.Second * 10)
	for _ = range ticker.C {
		json := heartbeatJson()
		key := fmt.Sprintf("pinky/%s/resources", boxId)

		redisClient.Setex(key, 20, json)
	}
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
