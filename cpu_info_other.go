//go:build !linux && !windows && !darwin

package main

import "runtime"

// cpuInfo stub for the *BSDs / illumos where fs_info is already a
// stub too — surface at least the core count so the section renders
// something meaningful. Model + load stay empty.
func cpuInfo() (CPUInfo, bool) {
	c := runtime.NumCPU()
	if c <= 0 {
		return CPUInfo{}, false
	}
	return CPUInfo{LogicalCores: c}, true
}
