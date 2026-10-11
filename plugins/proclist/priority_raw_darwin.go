//go:build darwin

package proclist

// rawPriorityToNice converts x/sys/unix.Getpriority's raw return to a true
// nice value. Unlike Linux (priority_raw_linux.go), Darwin's x/sys/unix
// wrapper calls into libSystem's own getpriority(3) through a dlsym
// trampoline (zsyscall_darwin_*.go), not a raw kernel syscall -- it already
// returns the true nice value, the same as glibc does on Linux, just
// reached through a different path. No offset to undo here.
func rawPriorityToNice(raw int) int { return raw }
