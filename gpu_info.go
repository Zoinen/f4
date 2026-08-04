package main

// GPUInfo is what the InfoPanel's GPU section renders. Systems with
// dGPU + iGPU return two entries. Empty slice → section is hidden
// entirely (macOS, headless *BSD, containers with no /dev/dri).
type GPUInfo struct {
	Model  string
	Driver string // kernel driver on Linux, DriverDesc/Provider on Windows; "" if unknown
}
