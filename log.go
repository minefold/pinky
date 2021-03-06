package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Logs as data!
// thanks to @mmcgrana 
// http://blog.librato.com/2012/06/sf-metrics-meetup-videos-visibility-at.html

type DataLogger interface {
	Info(data map[string]interface{})
	Error(err error, data map[string]interface{})
	Out(level string, data map[string]interface{})
}

type Logger struct {
	MetaData map[string]interface{}
}

func NewLog(metadata map[string]interface{}) DataLogger {
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
	if os.Getenv("LOGFMT") == "json" {
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
	attrs := make([]string, 0)

	for k, v := range data {
		if k != "ts" && k != "level" && k != "event" && v != nil {
			var s string
			switch v.(type) {
			case *json.RawMessage:
				msg := v.(*json.RawMessage)
				if msg != nil {
					var m interface{}
					json.Unmarshal(*msg, &m)
					s = fmt.Sprintf("%v", m)
				}

			default:
				s = fmt.Sprintf("%v", v)
			}

			if s != "" {
				if strings.Contains(s, " ") {
					s = strconv.Quote(s)
				}

				attrs = append(attrs, fmt.Sprintf("%s=%v", k, s))
			}
		}
	}

	sort.Strings(attrs)
	fmt.Println(fmt.Sprintf("%s [%s]", data["ts"], data["level"]), data["event"], strings.Join(attrs, " "))
}
