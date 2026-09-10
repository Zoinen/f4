package sysinfo

// GPUInfo is what the InfoPanel's GPU section renders. Systems with
// dGPU + iGPU return two entries. Empty slice → section is hidden
// entirely (macOS, headless *BSD, containers with no /dev/dri).
type GPUInfo struct {
	Model string
	// ModelKey names a message catalogue entry to render instead of Model.
	// A probe that has no vendor string to report — WSL passthrough without
	// interop is the one case — describes the adapter with a key, because
	// resolving it here would mean the hardware probes carry the catalogue.
	// Empty for a real model name, which is the normal case.
	ModelKey string
	Driver   string // kernel driver on Linux, DriverDesc/Provider on Windows; "" if unknown
}
