//go:build linux

package proclist

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Supported reports whether this build can collect a real process list.
func Supported() bool { return true }

// clockTicksPerSec is USER_HZ, the unit /proc/[pid]/stat reports utime and
// stime in. Linux has reported 100 here on every mainstream architecture for
// decades -- it is glibc's sysconf(_SC_CLK_TCK) default, and every other
// /proc-reading tool (ps, top, gopsutil's process package) assumes it
// without calling sysconf either. Hardcoding it avoids a cgo dependency for
// a value that in practice never varies.
const clockTicksPerSec = 100

// collect reads every /proc/[pid] entry currently present and returns one
// sample per process it could read. A process that exits between the
// directory listing and collect's own read of it -- a normal race for a
// periodic scanner like this one -- is skipped rather than treated as an
// error, and so is a process this one lacks permission to read.
func (c *collector) collect() ([]sample, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("proclist: read /proc: %w", err)
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	seen := make(map[int]bool, len(entries))
	samples := make([]sample, 0, len(entries))
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		raw, ok := readProcess(pid)
		if !ok {
			continue
		}
		seen[pid] = true

		ticks := raw.utime + raw.stime
		cpuPercent := 0.0
		if prev, ok := c.prev[pid]; ok && ticks >= prev.ticks {
			if dt := now.Sub(prev.at).Seconds(); dt > 0 {
				cpuPercent = float64(ticks-prev.ticks) / clockTicksPerSec / dt * 100
			}
		}
		c.prev[pid] = tickSample{ticks: ticks, at: now}

		samples = append(samples, sample{
			pid:        pid,
			name:       raw.name,
			rssKiB:     raw.rssKiB,
			cpuPercent: cpuPercent,
		})
	}

	// Bound memory: forget ticks for processes that no longer exist.
	for pid := range c.prev {
		if !seen[pid] {
			delete(c.prev, pid)
		}
	}
	return samples, nil
}

// rawProcess is what one /proc/[pid] read produces before collect turns it
// into a sample with a CPU%.
type rawProcess struct {
	name   string
	utime  uint64
	stime  uint64
	rssKiB uint64
}

func readProcess(pid int) (rawProcess, bool) {
	statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return rawProcess{}, false
	}
	name, utime, stime, ok := parseProcStat(string(statData))
	if !ok {
		return rawProcess{}, false
	}
	raw := rawProcess{name: name, utime: utime, stime: stime}
	if statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid)); err == nil {
		raw.rssKiB, _ = parseVmRSSKiB(string(statusData))
	}
	return raw, true
}

// parseProcStat extracts the comm (process name) and the utime/stime fields
// (14 and 15) from the content of /proc/[pid]/stat. The name is delimited by
// the first '(' and the last ')' in the line rather than split on spaces,
// because comm itself can contain spaces or parentheses (for example the
// kernel's own "(sd-pam)" helper, whose comm is literally "sd-pam" wrapped
// in one more pair of parens by the kernel).
func parseProcStat(line string) (name string, utime, stime uint64, ok bool) {
	open := strings.IndexByte(line, '(')
	closeParen := strings.LastIndexByte(line, ')')
	if open < 0 || closeParen < open {
		return "", 0, 0, false
	}
	name = line[open+1 : closeParen]

	fields := strings.Fields(line[closeParen+1:])
	// fields[0] is stat field 3 (state); stat field N is fields[N-3].
	const utimeField, stimeField = 14, 15
	utimeIdx, stimeIdx := utimeField-3, stimeField-3
	if len(fields) <= stimeIdx {
		return name, 0, 0, false
	}
	u, err1 := strconv.ParseUint(fields[utimeIdx], 10, 64)
	s, err2 := strconv.ParseUint(fields[stimeIdx], 10, 64)
	if err1 != nil || err2 != nil {
		return name, 0, 0, false
	}
	return name, u, s, true
}

// parseVmRSSKiB extracts VmRSS from the content of /proc/[pid]/status, e.g.
// "VmRSS:      1234 kB". A status block without a VmRSS line -- typically a
// kernel thread, which has no address space to report -- reports ok=false,
// and the caller leaves the field at zero.
func parseVmRSSKiB(status string) (uint64, bool) {
	for _, line := range strings.Split(status, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "VmRSS:")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, false
		}
		v, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}
