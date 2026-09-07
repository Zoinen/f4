//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	navigationBenchmarkKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	navigationBenchmarkQueryPerformanceCounter   = navigationBenchmarkKernel32.NewProc("QueryPerformanceCounter")
	navigationBenchmarkQueryPerformanceFrequency = navigationBenchmarkKernel32.NewProc("QueryPerformanceFrequency")
	navigationBenchmarkPerformanceFrequency      = navigationBenchmarkReadPerformanceFrequency()
)

func navigationBenchmarkReadPerformanceFrequency() int64 {
	var frequency int64
	ok, _, _ := navigationBenchmarkQueryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&frequency)))
	if ok == 0 || frequency <= 0 {
		return 0
	}
	return frequency
}

// navigationBenchmarkPerformanceCounterNs uses the same QueryPerformanceCounter
// epoch and scale as MSVC's std::chrono::steady_clock. Dividing before
// multiplying keeps ordinary system uptimes from overflowing int64.
func navigationBenchmarkPerformanceCounterNs(counter, frequency int64) int64 {
	if counter < 0 || frequency <= 0 {
		return 0
	}
	seconds := counter / frequency
	remainder := counter % frequency
	return seconds*1_000_000_000 + remainder*1_000_000_000/frequency
}

func navigationBenchmarkMonotonicNs() int64 {
	if navigationBenchmarkPerformanceFrequency <= 0 {
		return 0
	}
	var counter int64
	ok, _, _ := navigationBenchmarkQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&counter)))
	if ok == 0 {
		return 0
	}
	return navigationBenchmarkPerformanceCounterNs(counter, navigationBenchmarkPerformanceFrequency)
}
