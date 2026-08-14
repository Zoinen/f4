//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSessionDir_Isolation(t *testing.T) {
	dir := sessionDir()
	expectedSuffix := fmt.Sprintf("f4-sessions-%d", os.Getuid())

	if filepath.Base(dir) != expectedSuffix {
		t.Errorf("sessionDir() = %q; want suffix %q", dir, expectedSuffix)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("sessionDir was not created: %v", err)
	}

	if info.Mode().Perm() != 0700 {
		t.Errorf("sessionDir permissions = %v; want 0700", info.Mode().Perm())
	}
}

// TestClearNonBlock_ClearsFlag guards the OpenBSD 7.5+ regression described
// in PORTABILITY_BSD.md, 4.4: the original code called
// syscall.Syscall(syscall.SYS_FCNTL, ...) directly, an indirect syscall that
// OpenBSD 7.5+ no longer supports (golang/go#63900) — it returns ENOSYS and
// silently leaves O_NONBLOCK set. clearNonBlock instead goes through
// unix.FcntlInt, which uses the libc fcntl(2) stub and keeps working there.
//
// This test can't reproduce the ENOSYS behavior itself (that only happens on
// real OpenBSD 7.5+), but it pins down the contract clearNonBlock must
// satisfy everywhere: given a fd with O_NONBLOCK set, F_GETFL must no longer
// report it afterwards.
func TestClearNonBlock_ClearsFlag(t *testing.T) {
	var p [2]int
	if err := syscall.Pipe(p[:]); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	readEnd, writeEnd := p[0], p[1]
	defer syscall.Close(readEnd)
	defer syscall.Close(writeEnd)

	if _, err := unix.FcntlInt(uintptr(readEnd), unix.F_SETFL, unix.O_NONBLOCK); err != nil {
		t.Fatalf("set O_NONBLOCK: %v", err)
	}
	flags, err := unix.FcntlInt(uintptr(readEnd), unix.F_GETFL, 0)
	if err != nil {
		t.Fatalf("F_GETFL: %v", err)
	}
	if flags&unix.O_NONBLOCK == 0 {
		t.Fatalf("test setup broken: O_NONBLOCK not observed as set")
	}

	f := os.NewFile(uintptr(readEnd), "pipe-read")
	clearNonBlock(f)

	flags, err = unix.FcntlInt(uintptr(readEnd), unix.F_GETFL, 0)
	if err != nil {
		t.Fatalf("F_GETFL after clearNonBlock: %v", err)
	}
	if flags&unix.O_NONBLOCK != 0 {
		t.Fatalf("O_NONBLOCK still set after clearNonBlock")
	}
}

// mechanism behind the #429 investigation (PORTABILITY_BSD.md, 4.1): fds
// received via SCM_RIGHTS carry no FD_CLOEXEC, so a child process spawned
// afterwards (e.g. the built-in terminal's shell, via initPTY) inherits them
// across fork+exec unless explicitly flagged. If that child outlives the
// daemon, it keeps notifyPipe's write end open and runClient's blocking read
// on it never returns — a daemon crash then looks like an indefinite hang
// instead of a clean, fast exit.
//
// This test proves the negative directly: a pipe write end is flagged with
// setCloseOnExec, a long-lived child is forked, our own copy of the write
// end is closed, and the read end must then see EOF immediately — meaning
// no other process (i.e. not the child) is still holding the write end open.
// Without the CLOEXEC flag this read blocks instead, because the forked
// child holds its own copy for as long as it runs.
func TestSetCloseOnExec_NotInheritedByChild(t *testing.T) {
	var p [2]int
	if err := syscall.Pipe(p[:]); err != nil {
		t.Fatalf("pipe: %v", err)
	}
	readEnd, writeEnd := p[0], p[1]
	defer syscall.Close(readEnd)

	setCloseOnExec([]int{writeEnd})

	// A child that outlives this test's assertions if it inherited writeEnd.
	proc, err := os.StartProcess("/bin/sleep", []string{"sleep", "5"}, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	defer func() {
		proc.Kill()
		proc.Wait()
	}()

	// We are now the only process that should hold writeEnd open. Closing it
	// must make the read end observe EOF right away.
	if err := syscall.Close(writeEnd); err != nil {
		t.Fatalf("close(writeEnd): %v", err)
	}

	if _, err := unix.FcntlInt(uintptr(readEnd), unix.F_SETFL, unix.O_NONBLOCK); err != nil {
		t.Fatalf("set O_NONBLOCK on readEnd: %v", err)
	}
	buf := make([]byte, 1)
	n, err := syscall.Read(readEnd, buf)
	if n != 0 || err != nil {
		t.Fatalf("read after close(writeEnd) = (%d, %v); want (0, nil) EOF — "+
			"a non-EOF result means the forked child still holds the write "+
			"end open, i.e. FD_CLOEXEC did not take effect", n, err)
	}
}
