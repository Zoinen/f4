//go:build !linux && !windows

package sysinfo

// MemInfo stub for non-Linux unices. Linux's sysinfo(2) equivalent on
// the BSDs is a mix of sysctl / kstat / vm.stats calls that don't
// have a single portable helper in the standard library. The info
// panel silently omits the memory section here rather than carry
// per-BSD ports for now.
func Mem() (MemInfo, bool) {
	return MemInfo{}, false
}
