//go:build linux

package sysinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseCacheSize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want uint64
	}{
		{"empty", "", 0},
		{"kilobytes", "32K", 32 * 1024},
		{"lowercase kilobytes", "32k", 32 * 1024},
		{"megabytes", "12M", 12 * 1024 * 1024},
		{"lowercase megabytes", "12m", 12 * 1024 * 1024},
		{"gigabytes", "1G", 1024 * 1024 * 1024},
		{"lowercase gigabytes", "1g", 1024 * 1024 * 1024},
		{"bare number, no suffix", "4096", 4096},
		{"garbage", "not-a-size", 0},
		{"garbage with recognized suffix", "abcK", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parseCacheSize(c.in); got != c.want {
				t.Errorf("parseCacheSize(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestReadUintFile(t *testing.T) {
	dir := t.TempDir()

	valid := filepath.Join(dir, "valid")
	if err := os.WriteFile(valid, []byte(" 42 \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := readUintFile(valid); !ok || got != 42 {
		t.Errorf("readUintFile(valid) = (%d, %v), want (42, true)", got, ok)
	}

	invalid := filepath.Join(dir, "invalid")
	if err := os.WriteFile(invalid, []byte("not-a-number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, ok := readUintFile(invalid); ok || got != 0 {
		t.Errorf("readUintFile(invalid content) = (%d, %v), want (0, false)", got, ok)
	}

	missing := filepath.Join(dir, "does-not-exist")
	if got, ok := readUintFile(missing); ok || got != 0 {
		t.Errorf("readUintFile(missing) = (%d, %v), want (0, false)", got, ok)
	}
}

func TestReadLoadAvg(t *testing.T) {
	// /proc/loadavg always exists on a real Linux kernel, including the
	// GitHub Actions ubuntu-latest runner this test runs on in CI, so this
	// exercises the real success path rather than a mock.
	got, ok := readLoadAvg()
	if !ok {
		t.Fatal("readLoadAvg() ok = false, want true on a Linux CI runner")
	}
	for i, v := range got {
		if v < 0 {
			t.Errorf("readLoadAvg()[%d] = %v, want >= 0", i, v)
		}
	}
}

func TestParseProcCPUInfo(t *testing.T) {
	// parseProcCPUInfo reads the real /proc/cpuinfo (it has no path
	// injection seam), so this asserts the invariants the CI Linux runner
	// can actually guarantee rather than exact values.
	var info CPUInfo
	parseProcCPUInfo(&info)
	if info.PhysicalCores < 0 {
		t.Errorf("PhysicalCores = %d, want >= 0", info.PhysicalCores)
	}
	if info.FreqMHz < 0 {
		t.Errorf("FreqMHz = %d, want >= 0", info.FreqMHz)
	}
}

func TestCPU(t *testing.T) {
	info, ok := CPU()
	if !ok {
		t.Fatal("CPU() ok = false, want true on a Linux CI runner")
	}
	if info.LogicalCores != runtime.NumCPU() {
		t.Errorf("LogicalCores = %d, want runtime.NumCPU() = %d", info.LogicalCores, runtime.NumCPU())
	}
	// A second call must reuse the cached static part (cpuStaticOnce) but
	// still refresh LoadAvg/HasLoad every time.
	info2, ok2 := CPU()
	if !ok2 {
		t.Fatal("second CPU() call ok = false")
	}
	if info2.Model != info.Model || info2.LogicalCores != info.LogicalCores {
		t.Errorf("static CPU fields changed between calls: %+v vs %+v", info, info2)
	}
}

func TestReadLinuxCaches(t *testing.T) {
	// Best-effort: just make sure it does not panic and only ever fills
	// levels 1..4, regardless of what this runner's real /sys exposes.
	var info CPUInfo
	readLinuxCaches(&info)
	for i, b := range info.CacheBytes {
		if b > 0 && (i+1) > 4 {
			t.Errorf("CacheBytes has a value at out-of-range level %d", i+1)
		}
	}
}
