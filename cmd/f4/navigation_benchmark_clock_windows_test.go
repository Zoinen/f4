//go:build windows

package main

import (
	"testing"
	"time"
)

func TestNavigationBenchmarkPerformanceCounterConversion(t *testing.T) {
	const frequency = int64(10_000_000)
	const counter = int64(123*frequency + frequency/4)
	if got, want := navigationBenchmarkPerformanceCounterNs(counter, frequency), int64(123_250_000_000); got != want {
		t.Fatalf("counter conversion = %d, want %d", got, want)
	}
}

func TestNavigationBenchmarkWindowsClockAdvances(t *testing.T) {
	if navigationBenchmarkPerformanceFrequency <= 0 {
		t.Fatal("QueryPerformanceFrequency failed")
	}
	before := navigationBenchmarkMonotonicNs()
	time.Sleep(2 * time.Millisecond)
	after := navigationBenchmarkMonotonicNs()
	if before <= 0 || after <= before {
		t.Fatalf("QPC timestamps did not advance: before=%d after=%d", before, after)
	}
}
