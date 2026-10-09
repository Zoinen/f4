package proclist

import (
	"sync"
	"time"
)

// sample is one process's data for one collection pass. Every platform's
// collect() (collector_linux.go, collector_windows.go, collector_darwin.go)
// fills the same four fields; only how it gets them differs.
type sample struct {
	pid        int
	name       string
	rssKiB     uint64
	cpuPercent float64
}

// tickSample is what collector remembers about a process between two
// collect calls, so the next one can turn a platform's cumulative
// kernel+user CPU-time counter into a CPU% for the interval between them.
// The unit differs per platform (Linux: USER_HZ clock ticks; Windows:
// FILETIME's 100ns units; macOS: nanoseconds), so each collect
// implementation picks its own divisor; ticks itself is opaque here.
type tickSample struct {
	ticks uint64
	at    time.Time
}

// collector accumulates the CPU-time deltas collect needs across calls. Its
// zero value is not usable; construct one with newCollector. The
// accumulator itself does not vary by platform -- only the collect method
// that fills it does, one per collector_<os>.go.
type collector struct {
	mu   sync.Mutex
	prev map[int]tickSample
}

func newCollector() *collector {
	return &collector{prev: make(map[int]tickSample)}
}
