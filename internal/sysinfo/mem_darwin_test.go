//go:build darwin

package sysinfo

import "testing"

func TestDarwinMemoryAvailable(t *testing.T) {
	info, ok := Mem()
	if !ok || info.Total == 0 {
		t.Fatal("macOS physical memory information missing")
	}
	if info.Free > info.Total || info.SwapFree > info.SwapTotal {
		t.Fatalf("invalid capacity: %+v", info)
	}
	if info.LoadPercent < 0 || info.LoadPercent > 100 {
		t.Fatalf("invalid memory load: %+v", info)
	}
	t.Logf("[FIX:darwin-info] memory %+v", info)
}

func TestDarwinMemoryPageAccounting(t *testing.T) {
	for _, tc := range []struct {
		name           string
		free, inactive uint32
		wantFree       uint64
		wantLoad       int
	}{
		{name: "free and inactive", free: 1, inactive: 2, wantFree: 300, wantLoad: 70},
		{name: "clamped during sampling", free: 20, wantFree: 1000, wantLoad: 0},
		{name: "full", wantLoad: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := darwinMemoryFromPages(1000, 100, tc.free, tc.inactive)
			if info.Free != tc.wantFree || info.LoadPercent != tc.wantLoad {
				t.Fatalf("page accounting = %+v", info)
			}
		})
	}
}
