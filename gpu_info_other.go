//go:build !linux && !windows && !darwin

package main

// gpuInfo stub for *BSD / illumos. Same rationale as fs_info_other:
// no per-BSD ports carried until someone actually asks.
func gpuInfo() ([]GPUInfo, bool) {
	return nil, false
}
