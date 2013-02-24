package main

import (
	"encoding/json"
)

type ServerEvent map[string]*json.RawMessage

func ParseServerEvent(line []byte) (event ServerEvent, err error) {
	err = json.Unmarshal(line, &event)
	return
}

func (e *ServerEvent) Type() string {
	var str string
	var event = *e
	json.Unmarshal(*event["event"], &str)
	return str
}
