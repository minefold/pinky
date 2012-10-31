package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

func runtimeDump() {
	for {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		fmt.Printf(FgYellow+"%d, %02f, %d\n"+Reset,
			runtime.NumGoroutine(),
			float32(stats.Alloc)/1024.0/1024.0,
			stats.NumGC)
		time.Sleep(time.Second)
		runtime.GC()
	}
}

func memoryDump() {
	tick := time.NewTicker(10 * time.Second)
	i := 0
	for _ = range tick.C {
		memProf, err := os.Create(fmt.Sprintf("mem%d.prof", i))
		if err != nil {
			panic(err)
		}
		pprof.WriteHeapProfile(memProf)
		memProf.Close()
		i++
	}
}
