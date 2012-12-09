package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type hbJson struct {
	FreeDiskMb int `json:"freeDiskMb"`
	FreeRamMb  int `json:"freeRamMb"`
	IdleCpu    int `json:"idleCpu"`
}

func heartbeat(serverRoot string) {
	heartbeatJson(serverRoot)
	ticker := time.NewTicker(time.Second * 10)
	for _ = range ticker.C {
		json, err := heartbeatJson(serverRoot)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event": "heartbeat_failed",
			})
		}

		key := fmt.Sprintf("pinky:%s:heartbeat", boxId)
		redisClient.Setex(key, 20, json)
	}
}

func heartbeatJson(serverRoot string) ([]byte, error) {
	if err := recover(); err != nil {
		// TODO bugsnag
		fmt.Println("HEARTBEAT ERROR", err)
	}

	vmStat, err := GetVmStat()
	if err != nil {
		return nil, err
	}
	diskUsage, err := CollectDiskUsage(serverRoot)
	if err != nil {
		return nil, err
	}

	v := hbJson{
		FreeDiskMb: diskUsage.MbTotal - diskUsage.MbUsed,
		FreeRamMb:  vmStat.FreeMb,
		IdleCpu:    vmStat.IdleCpu,
	}

	json, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	return json, nil
}
