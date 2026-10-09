//go:build solaris || illumos

package terminal

// PreservesLogicalLines is always true for a Unix pty: the kernel passes the
// child's bytes through, and nothing between the child and f4 wraps them.
func (p *SolarisPTY) PreservesLogicalLines() bool { return true }
