//go:build linux

package sysinfo

import "testing"

func TestMem(t *testing.T) {
	// syscall.Sysinfo(2) always succeeds on a real Linux kernel, including
	// the ubuntu-latest GitHub Actions runner this test runs on in CI, so
	// this exercises Mem()'s real success path and its derived
	// LoadPercent computation against actual kernel-reported values.
	info, ok := Mem()
	if !ok {
		t.Fatal("Mem() ok = false, want true on a Linux CI runner")
	}
	if info.Total == 0 {
		t.Error("Total = 0, want a positive total memory size")
	}
	if info.Free > info.Total+info.SwapTotal {
		t.Errorf("Free (%d) exceeds Total+SwapTotal (%d)", info.Free, info.Total+info.SwapTotal)
	}
	if info.LoadPercent < 0 || info.LoadPercent > 100 {
		t.Errorf("LoadPercent = %d, want in [0, 100]", info.LoadPercent)
	}
}
