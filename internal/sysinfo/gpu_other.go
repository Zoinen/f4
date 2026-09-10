//go:build !linux && !windows && !darwin

package sysinfo

// gpuInfo stub for *BSD / illumos. Same rationale as fs_info_other:
// no per-BSD ports carried until someone actually asks.
func GPU() ([]GPUInfo, bool) {
	return nil, false
}
