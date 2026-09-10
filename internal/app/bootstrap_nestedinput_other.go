//go:build !windows

package app

// prepareNestedConsoleInput does nothing where there is no console host
// between f4 and its input: a pty hands the bytes over as they were written.
func prepareNestedConsoleInput() {}
