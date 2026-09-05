//go:build linux

package main

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var (
	cpuStaticOnce sync.Once
	cachedCPU     CPUInfo
)

func cpuInfo() (CPUInfo, bool) {
	cpuStaticOnce.Do(func() {
		cachedCPU = readStaticLinuxCPU()
	})
	info := cachedCPU
	if la, ok := readLoadAvg(); ok {
		info.LoadAvg = la
		info.HasLoad = true
	}
	if info.Model == "" && info.LogicalCores == 0 && info.PhysicalCores == 0 &&
		info.FreqMHz == 0 && info.CacheBytes == [4]uint64{} && !info.HasLoad {
		return CPUInfo{}, false
	}
	return info, true
}

func readStaticLinuxCPU() CPUInfo {
	info := CPUInfo{LogicalCores: runtime.NumCPU()}
	parseProcCPUInfo(&info)
	if info.FreqMHz == 0 {
		if khz, ok := readUintFile("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq"); ok {
			info.FreqMHz = int(khz / 1000)
		}
	}
	readLinuxCaches(&info)
	return info
}

// parseProcCPUInfo pulls model name, cpu MHz and physical-core count
// from /proc/cpuinfo. Physical cores are counted as the number of
// distinct (physical id, core id) pairs — the same trick lscpu uses.
// On single-socket kernels that omit physical id we fall back to
// "cpu cores" from the first processor block, which is per-socket.
func parseProcCPUInfo(info *CPUInfo) {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return
	}
	defer f.Close()

	pairs := map[string]struct{}{}
	physID, coreID := "", ""
	haveTopology := false
	firstBlockCores := 0
	sockets := map[string]struct{}{}

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		colon := strings.IndexByte(line, ':')
		if colon < 0 {
			if physID != "" && coreID != "" {
				pairs[physID+"/"+coreID] = struct{}{}
				sockets[physID] = struct{}{}
				haveTopology = true
			}
			physID, coreID = "", ""
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "model name", "Model", "cpu model":
			if info.Model == "" {
				info.Model = val
			}
		case "cpu MHz":
			if info.FreqMHz == 0 {
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					info.FreqMHz = int(f + 0.5)
				}
			}
		case "physical id":
			physID = val
		case "core id":
			coreID = val
		case "cpu cores":
			if firstBlockCores == 0 {
				firstBlockCores, _ = strconv.Atoi(val)
			}
		}
	}
	if physID != "" && coreID != "" {
		pairs[physID+"/"+coreID] = struct{}{}
		sockets[physID] = struct{}{}
		haveTopology = true
	}
	switch {
	case haveTopology && len(pairs) > 0:
		info.PhysicalCores = len(pairs)
	case firstBlockCores > 0:
		// Single-socket fallback: kernels that don't emit
		// physical id still print cpu cores per-processor.
		info.PhysicalCores = firstBlockCores
	}
}

// readLinuxCaches scans /sys/devices/system/cpu/cpu0/cache and
// aggregates unified/data-side caches per level. We use cpu0 as the
// witness — cores within a socket share caches from L2 upward, and
// asymmetric big.LITTLE cores (Alder Lake) still list the same L1
// per cpu. Sizes come as "32K" / "12M" strings.
func readLinuxCaches(info *CPUInfo) {
	entries, err := filepath.Glob("/sys/devices/system/cpu/cpu0/cache/index*")
	if err != nil {
		return
	}
	for _, e := range entries {
		level := 0
		if s, err := os.ReadFile(filepath.Join(e, "level")); err == nil {
			level, _ = strconv.Atoi(strings.TrimSpace(string(s)))
		}
		if level < 1 || level > 4 {
			continue
		}
		typ := ""
		if s, err := os.ReadFile(filepath.Join(e, "type")); err == nil {
			typ = strings.TrimSpace(string(s))
		}
		// L1 has separate Data / Instruction; sum both under "L1".
		// L2+ are unified — we take the size once.
		if level > 1 && typ == "Instruction" {
			continue
		}
		sizeStr := ""
		if s, err := os.ReadFile(filepath.Join(e, "size")); err == nil {
			sizeStr = strings.TrimSpace(string(s))
		}
		bytes := parseCacheSize(sizeStr)
		if bytes == 0 {
			continue
		}
		info.CacheBytes[level-1] += bytes
	}
}

func parseCacheSize(s string) uint64 {
	if s == "" {
		return 0
	}
	mult := uint64(1)
	switch s[len(s)-1] {
	case 'K', 'k':
		mult = 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1024 * 1024
		s = s[:len(s)-1]
	case 'G', 'g':
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return n * mult
}

func readUintFile(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// readLoadAvg parses the first three numbers from /proc/loadavg.
func readLoadAvg() ([3]float64, bool) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return [3]float64{}, false
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return [3]float64{}, false
	}
	var out [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			return [3]float64{}, false
		}
		out[i] = v
	}
	return out, true
}
