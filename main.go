package main

import (
	"fmt"
	"os"
	"os/exec"
)

var conf *PinkyConfig

func main() {
	conf = InitPinkyConfig()

	// allocate 200 ports starting at 10000 with a 100 port gap
	portPool := NewIntPool(10000, 200, 100, []int{})

	mounts := map[string]string{
		"funpack": "http://party-cloud-production.s3.amazonaws.com/funpacks/slugs/50a976ec7aae5741bb000001.tar.gz",
		"working": "http://party-cloud-production.s3.amazonaws.com/worlds/51265a0726da4f000200003a/51265a0726da4f000200003a.1362528923.tar.lzo",
	}

	cmd := exec.Command("/funpack/bin/run")
	cmd.Dir = "/working"
	cmd.Env = []string{
		"PORT=" + fmt.Sprintf("%d", <-portPool),
		"RAM=768",
		"DATA=" + `{
			"name":"superfun",
			"settings":{"ops":"whatupdave"}
		}`,
		"BUNDLE_GEMFILE=/funpack/Gemfile",
		"SHARED_DIR=/shared",
	}

	dyno := NewDyno(Uuid(), cmd, mounts)

	go func() {
		for line := range dyno.Stdout {
			event, err := ParseServerEvent(line)
			if err != nil {
				panic(err)
			} else {
				switch event.Type() {
				case "started":
					dyno.Stdin <- []byte("stop\n")
				}

				fmt.Println(event.Map())
			}

		}
	}()

	dyno.Start()
}

func Uuid() string {
	f, _ := os.Open("/dev/urandom")
	b := make([]byte, 16)
	f.Read(b)
	f.Close()
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
