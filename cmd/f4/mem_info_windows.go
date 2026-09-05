//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// memoryStatusEx matches Windows' MEMORYSTATUSEX layout.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
)

// memInfo returns physical-memory info via GlobalMemoryStatusEx.
func memInfo() (MemInfo, bool) {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return MemInfo{}, false
	}
	return MemInfo{
		Total:       m.TotalPhys,
		Free:        m.AvailPhys,
		SwapTotal:   m.TotalPageFile,
		SwapFree:    m.AvailPageFile,
		LoadPercent: int(m.MemoryLoad),
	}, true
}
