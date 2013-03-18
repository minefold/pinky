package main

import (
	"encoding/json"
)

type ServerEvent map[string]interface{}

func ParseServerEvent(line []byte) (event ServerEvent, err error) {
	err = json.Unmarshal(line, &event)
	return
}

func (e *ServerEvent) Type() string {
	return (*e)["event"].(string)
}
