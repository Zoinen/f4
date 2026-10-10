//go:build !windows && !freebsd && !linux

package editor

import "io"

// inheritsControllingTTY is only implemented for Linux, where f4#1721 was
// reported (TIOCGSID is not available everywhere). Elsewhere the editor keeps
// getting its own session, as before.
func inheritsControllingTTY(io.Reader) bool { return false }
