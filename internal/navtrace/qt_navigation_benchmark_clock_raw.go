//go:build darwin || linux

package navtrace

import (
	"golang.org/x/sys/unix"
)

func NavigationBenchmarkMonotonicNs() int64 {
	var ts unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC_RAW, &ts); err != nil {
		return 0
	}
	return int64(ts.Sec)*1_000_000_000 + int64(ts.Nsec)
}
