//go:build !windows && !linux

package main

import "os"

// newMockTTYSlave falls back to a plain temp file on Unix hosts other than
// Linux. CI only runs `go test` for linux and windows (see
// .github/workflows/build.yml), so this path exists solely for a
// contributor running `go test .` locally on, say, macOS or FreeBSD; it was
// already the behavior before Setctty was added to SolarisPTY.Run(), and a
// test exercising that ioctl may fail here the same way it would have
// everywhere before this file existed. Anyone wiring up a real pty for these
// hosts can extend this the way solaris_streams_mock_linux_test.go does for
// Linux.
func newMockTTYSlave() (*os.File, error) {
	f, err := os.CreateTemp("", "mock_pts_*")
	if err == nil {
		os.Remove(f.Name()) // Unlink immediately to prevent leaks
	}
	return f, err
}
