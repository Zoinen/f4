//go:build !windows && (!(linux || darwin || freebsd) || !(amd64 || arm64))

package main

// goffi does not build for these targets -- its ffi package needs Windows, or
// Linux/macOS on amd64/arm64 -- so f4 must not import it here just to
// preflight. The FFI-backed GUI backends are not built on these targets
// either, and each remaining backend performs its own platform checks, so
// reporting true keeps the preflight out of the way rather than blocking a
// backend that does work.
func ffiAvailableForGUI() bool {
	return true
}
