package main

import (
	"log"
	// "os"
	// "runtime/pprof"
	"testing"
	"time"
)

func performBackup() {
	log.Println("starting backup")
	time.Sleep(2 * time.Second)
	log.Println("finished backup")
}

func runTestBackups(stop chan bool) {
	ticker := time.NewTicker(5 * time.Second)

	for {
		select {
		case <-ticker.C:
			performBackup()

		case <-stop:
			ticker.Stop()
			return

		default:
			time.Sleep(1 * time.Second)
		}
	}
}

func TestCancelBackups(t *testing.T) {
	// f, _ := os.Create("./cipher.prof")
	// defer f.Close()

	// pprof.StartCPUProfile(os.Stdout)
	// defer pprof.StopCPUProfile()

	stop := make(chan bool) // stop automated backups

	go runTestBackups(stop)

	time.Sleep(6 * time.Second)

	log.Println("stopping backups")
	stop <- true
}
