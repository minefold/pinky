package main

import (
	"encoding/json"
	"fmt"
	"time"
)

type hbJson struct {
	BoxType    string `json:"instanceType"`
	FreeDiskMb int    `json:"freeDiskMb"`
	FreeRamMb  int    `json:"freeRamMb"`
	IdleCpu    int    `json:"idleCpu"`
}

func heartbeat(instanceType string, serverRoot string) {
	ticker := time.NewTicker(time.Second * 10)
	for _ = range ticker.C {
		json, err := heartbeatJson(instanceType, serverRoot)
		if err != nil {
			plog.Error(err, map[string]interface{}{
				"event": "heartbeat_failed",
			})
			return
		}

		key := fmt.Sprintf("pinky/%s/heartbeat", boxId)
		redisClient.Setex(key, 20, json)
	}
}

func heartbeatJson(instanceType string, serverRoot string) ([]byte, error) {
	vmStat, err := GetVmStat()
	if err != nil {
		return nil, err
	}
	diskUsage, err := CollectDiskUsage(serverRoot)
	if err != nil {
		return nil, err
	}

	v := hbJson{
		BoxType:    instanceType,
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
