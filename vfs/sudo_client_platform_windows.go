//go:build windows

package vfs

// Windows uses UAC elevation for operations that need administrator access.
// The Unix socket dispatcher cannot work there, so callers must not attempt
// to start it (in particular, this prevents an accidental `sudo` invocation).
func sudoClientSupported() bool { return false }
