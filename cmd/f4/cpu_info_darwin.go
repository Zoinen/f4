//go:build darwin

package main

import (
	"encoding/binary"
	"runtime"
	"sync"
	"syscall"
)

var (
	cpuStaticOnce sync.Once
	cachedCPU     CPUInfo
)

func cpuInfo() (CPUInfo, bool) {
	cpuStaticOnce.Do(func() {
		cachedCPU = readStaticDarwinCPU()
	})
	info := cachedCPU
	if la, ok := readLoadAvgDarwin(); ok {
		info.LoadAvg = la
		info.HasLoad = true
	}
	if info.Model == "" && info.LogicalCores == 0 && info.PhysicalCores == 0 &&
		info.FreqMHz == 0 && info.CacheBytes == [4]uint64{} && !info.HasLoad {
		return CPUInfo{}, false
	}
	return info, true
}

func readStaticDarwinCPU() CPUInfo {
	info := CPUInfo{LogicalCores: runtime.NumCPU()}
	if s, err := syscall.Sysctl("machdep.cpu.brand_string"); err == nil {
		info.Model = s
	}
	if n, ok := sysctlUint64("hw.physicalcpu"); ok {
		if cores, fits := boundedUint64ToInt(n); fits {
			info.PhysicalCores = cores
		}
	}
	// hw.cpufrequency returns nominal clock in Hz on Intel; on Apple
	// Silicon this key is absent (fixed-frequency P/E cores with
	// on-die scaling — no meaningful single number). Skip on error.
	if hz, ok := sysctlUint64("hw.cpufrequency"); ok && hz > 0 {
		if mhz, fits := boundedUint64ToInt(hz / 1_000_000); fits {
			info.FreqMHz = mhz
		}
	}
	// L1i and L1d live under separate keys — sum them into L1.
	l1i, _ := sysctlUint64("hw.l1icachesize")
	l1d, _ := sysctlUint64("hw.l1dcachesize")
	info.CacheBytes[0] = l1i + l1d
	if n, ok := sysctlUint64("hw.l2cachesize"); ok {
		info.CacheBytes[1] = n
	}
	if n, ok := sysctlUint64("hw.l3cachesize"); ok {
		info.CacheBytes[2] = n
	}
	return info
}

// sysctlUint64 wraps syscall.Sysctl for numeric keys. Darwin returns
// them as raw little-endian bytes; length is 4 or 8 depending on
// the key (hw.cpufrequency is 8 on 64-bit systems, hw.l*cachesize
// is 8, hw.physicalcpu is 4).
func sysctlUint64(name string) (uint64, bool) {
	raw, err := syscall.Sysctl(name)
	if err != nil {
		return 0, false
	}
	b := []byte(raw)
	switch len(b) {
	case 4:
		return uint64(binary.LittleEndian.Uint32(b)), true
	case 8:
		return binary.LittleEndian.Uint64(b), true
	}
	return 0, false
}

// readLoadAvgDarwin reads vm.loadavg. macOS returns a struct loadavg
// (fixpt_t ldavg[3]; long fscale). fscale is stable at 2048 on
// Darwin (sys/resource.h: FSCALE = 1<<11), so we skip decoding it —
// makes the parse layout-independent.
func readLoadAvgDarwin() ([3]float64, bool) {
	raw, err := syscall.Sysctl("vm.loadavg")
	if err != nil || len(raw) < 12 {
		return [3]float64{}, false
	}
	const fscale = 2048.0
	var out [3]float64
	for i := 0; i < 3; i++ {
		v := binary.LittleEndian.Uint32([]byte(raw[i*4 : i*4+4]))
		out[i] = float64(v) / fscale
	}
	return out, true
}
