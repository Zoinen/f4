//go:build !windows

package terminal

import (
	"context"
	"os/exec"
	"strings"
	"syscall"

	"github.com/unxed/f4/vfs"
)

func newLocalShellCommand(command string) *exec.Cmd {
	return exec.Command(GetSystemShell(), "-c", command) // #nosec G204 -- this is the intentional local shell command line.
}

func newLocalShellCommandContext(ctx context.Context, command string) *exec.Cmd {
	return exec.CommandContext(ctx, GetSystemShell(), "-c", command) // #nosec G204 -- this is the intentional local shell command line.
}

func localCommandDialect() vfs.CommandDialect { return vfs.CommandDialectPOSIX }

func localCommandEnvironment([]string) []string { return nil }

func normalizeCommandOutput(line []byte) string {
	return strings.ToValidUTF8(string(line), "\uFFFD")
}

func configureLocalProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func attachLocalProcessTree(cmd *exec.Cmd) localProcessTree {
	return unixLocalProcessTree{pid: cmd.Process.Pid}
}

type unixLocalProcessTree struct{ pid int }

func (p unixLocalProcessTree) Kill() error {
	return syscall.Kill(-p.pid, syscall.SIGKILL)
}

func (unixLocalProcessTree) Close() error { return nil }
