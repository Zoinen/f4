//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// TestPTYDescriptorsAreCloseOnExec guards the fix for the pty leak that
// exhausted the host's pty table.
//
// Go marks close-on-exec every descriptor it opens itself, but a raw
// unix.Open wrapped in os.NewFile keeps whatever flags the fd was created
// with, and forkExec closes nothing on its own. When the master was left
// without FD_CLOEXEC, every shell f4 spawned inherited a copy of its own
// master. The master then never dropped to zero references once f4 died, so
// the kernel never delivered SIGHUP to the slave side: the shell survived as
// an orphan holding its pty node forever. Enough f4 restarts walked the host
// into kern.tty.ptmx_max and the next allocation failed with ENXIO, which
// looks to the person like "openpty failed" and no terminal anywhere.
//
// Checking the flag is the whole regression: Run() installs the slave on the
// child's 0/1/2 with dup2, which clears FD_CLOEXEC on those copies, so the
// child still gets a working terminal either way. The flag only decides
// whether the surplus descriptors tag along.
func TestPTYDescriptorsAreCloseOnExec(t *testing.T) {
	pty, err := NewPTY()
	if err != nil {
		t.Skipf("PTY allocation unavailable in this environment: %v", err)
	}
	t.Cleanup(func() {
		if err := pty.Close(); err != nil {
			t.Errorf("close PTY: %v", err)
		}
	})

	for _, d := range []struct {
		name string
		file *os.File
	}{
		{"master", pty.Master},
		{"slave", pty.Slave},
	} {
		if d.file == nil {
			t.Fatalf("%s descriptor is nil", d.name)
		}
		flags, err := unix.FcntlInt(d.file.Fd(), unix.F_GETFD, 0)
		if err != nil {
			t.Fatalf("F_GETFD on %s: %v", d.name, err)
		}
		if flags&unix.FD_CLOEXEC == 0 {
			t.Errorf("%s descriptor lacks FD_CLOEXEC; a spawned shell would "+
				"inherit it and keep the pty alive after f4 exits", d.name)
		}
	}
}

// TestPTYChildDoesNotInheritMaster is the behavioural half: it spawns a child
// through the real Run() path and confirms the master is absent from the
// child's descriptor table. This is what the flag above buys, verified
// end to end rather than by proxy.
func TestPTYChildDoesNotInheritMaster(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof not available to inspect the child's descriptors")
	}

	pty, err := NewPTY()
	if err != nil {
		t.Skipf("PTY allocation unavailable in this environment: %v", err)
	}
	t.Cleanup(func() {
		if err := pty.Close(); err != nil {
			t.Errorf("close PTY: %v", err)
		}
	})

	// A bare sleep keeps the child parked without a shell of its own, so
	// every descriptor it holds came from the exec, not from job control.
	if err := pty.Run("/bin/sh", "-c", "exec sleep 30"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	pid := pty.Cmd.Process.Pid

	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Skipf("lsof refused to inspect pid %d: %v", pid, err)
	}

	// The child legitimately holds the slave on 0, 1 and 2. Holding the
	// multiplexer clone is the bug: that is the master.
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "/dev/ptmx") {
			t.Errorf("child inherited the pty master, which prevents the "+
				"hangup that reaps it when f4 dies:\n  %s", strings.TrimSpace(line))
		}
	}
}
