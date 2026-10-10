//go:build darwin

package proclist

import (
	"os"
	"testing"
	"time"
)

func TestCommStringTrimsAtNUL(t *testing.T) {
	buf := [17]byte{'b', 'a', 's', 'h', 0, 'g', 'a', 'r', 'b', 'a', 'g', 'e'}
	if got, want := commString(buf[:]), "bash"; got != want {
		t.Fatalf("commString = %q, want %q", got, want)
	}
}

func TestCommStringWithNoNUL(t *testing.T) {
	buf := []byte("nonulhere")
	if got, want := commString(buf), "nonulhere"; got != want {
		t.Fatalf("commString = %q, want %q", got, want)
	}
}

func TestCollectFindsTheCurrentProcess(t *testing.T) {
	c := newCollector()
	samples, err := c.collect()
	if err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	for _, s := range samples {
		if s.pid == pid {
			if s.name == "" {
				t.Fatalf("sample for the current process has an empty name: %#v", s)
			}
			return
		}
	}
	t.Fatalf("current process (pid %d) missing from %d samples", pid, len(samples))
}

func TestCollectComputesCPUPercentAcrossTwoSamples(t *testing.T) {
	c := newCollector()
	if _, err := c.collect(); err != nil {
		t.Fatal(err)
	}

	// Burn CPU deliberately so libproc's user+system nanosecond counters are
	// guaranteed to advance between samples; waiting on the clock alone
	// without doing any work would not.
	deadline := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(deadline) {
	}

	samples, err := c.collect()
	if err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	for _, s := range samples {
		if s.pid == pid {
			if s.cpuPercent <= 0 {
				t.Fatalf("cpuPercent = %v after 150ms of busy work, want > 0", s.cpuPercent)
			}
			return
		}
	}
	t.Fatalf("current process (pid %d) missing from the second sample", pid)
}
