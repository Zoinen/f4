package main

// MemInfo describes overall system memory. Fields are 0 if the OS
// couldn't supply them. Per-OS implementations of memInfo live in
// mem_info_{linux,windows,other}.go.
type MemInfo struct {
	Total, Free uint64 // bytes
	Shared      uint64
	Buffered    uint64
	SwapTotal   uint64
	SwapFree    uint64
	LoadPercent int
}
