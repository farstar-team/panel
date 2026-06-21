package systeminfo

import (
	"runtime"
	"time"
)

var startedAt = time.Now()

type Info struct {
	OS           string  `json:"os"`
	Architecture string  `json:"architecture"`
	GoVersion    string  `json:"go_version"`
	Goroutines   int     `json:"goroutines"`
	MemoryMB     float64 `json:"memory_mb"`
	Uptime       int64   `json:"uptime_seconds"`
}

func Read() Info {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return Info{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		GoVersion:    runtime.Version(),
		Goroutines:   runtime.NumGoroutine(),
		MemoryMB:     float64(memory.Alloc) / 1024 / 1024,
		Uptime:       int64(time.Since(startedAt).Seconds()),
	}
}
