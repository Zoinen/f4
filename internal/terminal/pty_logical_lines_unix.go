//go:build linux || darwin || freebsd || dragonfly || openbsd || netbsd

package terminal

// PreservesLogicalLines is always true for a Unix pty: the kernel passes the
// child's bytes through, and nothing between the child and f4 wraps them.
func (p *PTY) PreservesLogicalLines() bool { return true }
