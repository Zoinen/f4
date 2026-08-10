//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinPTYDescriptorsAreCloseOnExecAndParentReleasesSlave(t *testing.T) {
	pty, err := NewPTY()
	if err != nil {
		t.Fatal(err)
	}
	defer pty.Close()

	for name, file := range map[string]*os.File{"master": pty.Master, "slave": pty.Slave} {
		flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
		if err != nil {
			t.Fatalf("read %s descriptor flags: %v", name, err)
		}
		if flags&unix.FD_CLOEXEC == 0 {
			t.Fatalf("%s PTY descriptor can leak into the child", name)
		}
	}

	if err := pty.Run("/bin/sh"); err != nil {
		t.Fatal(err)
	}
	if pty.Slave != nil {
		t.Fatal("parent retained the PTY slave after starting the child")
	}
}

func TestDarwinPTYDirectorySyncCommandWorksInInteractiveZsh(t *testing.T) {
	expectPath := "/usr/bin/expect"
	zshPath := "/bin/zsh"
	if _, err := os.Stat(expectPath); err != nil {
		t.Skipf("interactive zsh regression test requires %s: %v", expectPath, err)
	}
	if _, err := os.Stat(zshPath); err != nil {
		t.Skipf("interactive zsh regression test requires %s: %v", zshPath, err)
	}

	target := filepath.Join(t.TempDir(), "directory with spaces and ' quote")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	syncCommand := unixPTYDirectorySyncCommand(target)

	const expectScript = `
set timeout 5
log_user 1
spawn /bin/zsh -dfi
expect "% "
send -- $env(F4_SYNC_COMMAND)
expect "% "
send -- "print -r -- __F4_SYNC_PWD__\$PWD\r"
expect "% "
send -- "exit\r"
expect eof
`
	cmd := exec.Command(expectPath, "-c", expectScript)
	cmd.Env = append(os.Environ(), "F4_SYNC_COMMAND="+syncCommand)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("interactive zsh sync failed: %v\n%s", err, output)
	}
	if strings.Contains(string(output), "cd: too many arguments") {
		t.Fatalf("zsh parsed the sync marker as cd arguments:\n%s", output)
	}
	if !strings.Contains(string(output), "__F4_SYNC_PWD__"+target) {
		t.Fatalf("zsh did not change to %q:\n%s", target, output)
	}
}
