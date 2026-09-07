//go:build windows || ((linux || darwin || freebsd) && (amd64 || arm64))

package main

import "github.com/go-webgpu/goffi/ffi"

// The constraint above tracks where goffi's ffi package actually builds:
// Windows on any architecture, and Linux, macOS or FreeBSD on amd64/arm64.
// Widening it (to all of linux, say) breaks the 32-bit and exotic Linux
// targets, where ffi has no implementation and the FFI-backed GUI backends
// are not built either.

func ffiAvailableForGUI() bool {
	return ffi.Available()
}
