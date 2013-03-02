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
	json.Unmarshal(*(*e)["event"], &str)
	return str
}

func (e *ServerEvent) Map() map[string]string {
	doc := map[string]string{}
	for k, v := range *e {
		var str string
		json.Unmarshal(*v, &str)
		doc[k] = str
	}
	return doc
}
