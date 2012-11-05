package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Logs as data!
// thanks to @mmcgrana 
// http://blog.librato.com/2012/06/sf-metrics-meetup-videos-visibility-at.html

type Logger struct {
	MetaData map[string]interface{}
}

func NewLog(metadata map[string]interface{}) *Logger {
	return &Logger{MetaData: metadata}
}

func (l *Logger) Info(data map[string]interface{}) {
	l.Out("info", data)
}

func (l *Logger) Error(err error, data map[string]interface{}) {
	data["error"] = err
	l.Out("error", data)
}

func (l *Logger) Out(level string, data map[string]interface{}) {
	data["level"] = level
	data["ts"] = time.Now().UTC().Format(time.RFC3339)

	for k, v := range l.MetaData {
		data[k] = v
	}

	for k, v := range data {
		data[k] = v
	}

	printLogMessage(data)
}

func printLogMessage(data map[string]interface{}) {
	if os.Getenv("LOGFMT") != "human" {
		printLogMessageJson(data)
	} else {
		printLogMessageHuman(data)
	}
}

func printLogMessageJson(data map[string]interface{}) {
	json, err := json.Marshal(data)
	if err != nil {
		fmt.Println(err, data)
	}

	fmt.Println(string(json))
}

func printLogMessageHuman(data map[string]interface{}) {
	attrs := make(map[string]interface{})

	for k, v := range data {
		if k != "ts" && k != "level" && k != "event" {
			attrs[k] = v
		}
	}
	fmt.Println(fmt.Sprintf("%s [%s]", data["ts"], data["level"]), data["event"], attrs)
}
