//go:build !darwin && !linux && !windows

package main

import "time"

var navigationBenchmarkProcessStart = time.Now()

// Platforms without CLOCK_MONOTONIC_RAW retain monotonic ordering within the
// Go process. macOS and Linux, the native Qt targets, use the shared raw clock.
func navigationBenchmarkMonotonicNs() int64 {
	return time.Since(navigationBenchmarkProcessStart).Nanoseconds()
}
