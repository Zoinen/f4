//go:build linux

package main

import "syscall"

// memInfo uses sysinfo(2) — same source far2l reads on Linux, so
// numbers line up between the two apps.
func memInfo() (MemInfo, bool) {
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err != nil {
		return MemInfo{}, false
	}
	// si.Unit is the multiplier for the *ram/*swap fields (usually 1
	// on Linux). Convert everything to bytes here so callers don't
	// have to remember.
	u := uint64(si.Unit)
	if u == 0 {
		u = 1
	}
	info := MemInfo{
		Total:     uint64(si.Totalram) * u,
		Free:      uint64(si.Freeram) * u,
		Shared:    uint64(si.Sharedram) * u,
		Buffered:  uint64(si.Bufferram) * u,
		SwapTotal: uint64(si.Totalswap) * u,
		SwapFree:  uint64(si.Freeswap) * u,
	}
	if denom := info.Total + info.SwapTotal; denom > 0 {
		freeAll := info.Free + info.SwapFree
		info.LoadPercent = int(100 - (freeAll*100)/denom)
	}
	return info, true
}
