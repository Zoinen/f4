//go:build linux

package proclist

// rawPriorityToNice converts x/sys/unix.Getpriority's raw return to a true
// nice value. On Linux, that wrapper calls the raw SYS_GETPRIORITY syscall
// directly, bypassing glibc: the kernel's own getpriority(2) cannot return a
// negative "success" value on that path (it would collide with the -1 error
// sentinel), so it returns 20-nice instead, in 0..39. glibc's own
// getpriority() library wrapper already undoes this before returning to its
// caller -- x/sys/unix does not go through glibc here, so the undoing has
// to happen on this side instead.
func rawPriorityToNice(raw int) int { return 20 - raw }
