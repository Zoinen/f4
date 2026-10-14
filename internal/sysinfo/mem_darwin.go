//go:build darwin

package sysinfo

import (
	"encoding/binary"

	"golang.org/x/sys/unix"
)

// Mem reads physical capacity, Mach page statistics and vm.swapusage.
// Free includes inactive reclaimable pages; speculative pages are already
// included in Mach free_count and must not be counted twice.
func Mem() (MemInfo, bool) {
	total, err := unix.SysctlUint64("hw.memsize")
	if err != nil || total == 0 {
		return MemInfo{}, false
	}
	pageSize, err := unix.SysctlUint32("hw.pagesize")
	if err != nil || pageSize == 0 {
		return MemInfo{}, false
	}
	api := loadDarwinSystemAPI()
	if api == nil {
		return MemInfo{}, false
	}
	// HOST_VM_INFO64 grows across OS releases. Supply ample room and consume
	// only the stable four-word prefix, respecting the returned count.
	var stats [128]uint32
	count := uint32(len(stats))
	host := api.hostSelf()
	defer api.deallocate(api.taskSelf(), host)
	status := api.hostStats(
		host,
		4,
		&stats[0],
		&count,
	)
	if status != 0 || count < 4 {
		darwinInfoDebug("host_statistics64 failed: %d, count %d", status, count)
		return MemInfo{}, false
	}
	info := darwinMemoryFromPages(
		total,
		uint64(pageSize),
		stats[0],
		stats[2],
	)
	if swap, err := unix.SysctlRaw("vm.swapusage"); err == nil && len(swap) >= 24 {
		info.SwapTotal = binary.LittleEndian.Uint64(swap[:8])
		info.SwapFree = min(binary.LittleEndian.Uint64(swap[8:16]), info.SwapTotal)
	}
	return info, true
}

func darwinMemoryFromPages(total, pageSize uint64, free, inactive uint32) MemInfo {
	pages := uint64(free) + uint64(inactive)
	available := total
	if pageSize == 0 || pages <= total/pageSize {
		available = pages * pageSize
	}
	info := MemInfo{Total: total, Free: available}
	if total > 0 {
		info.LoadPercent = int(float64(total-available) / float64(total) * 100)
	}
	return info
}
