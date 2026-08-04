//go:build darwin

package main

// gpuInfo on darwin: IOKit is the honest source but pulling it
// requires cgo, which we deliberately avoid. On Apple Silicon the
// GPU shares its identity with the CPU (Apple M2 Pro = both), and
// the info panel already renders CPU model. Rather than surface a
// noisy or misleading string, we skip the section. A follow-up can
// pull in an IOKit binding if anyone wants a distinct GPU row.
func gpuInfo() ([]GPUInfo, bool) {
	return nil, false
}
