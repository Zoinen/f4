//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/unxed/f4/vfs"
	"golang.org/x/sys/windows"
)

func newLocalShellCommand(command string) *exec.Cmd {
	return exec.Command(GetSystemShell(), "/D", "/V:OFF", "/S", "/C", command)
}

func localCommandDialect() vfs.CommandDialect { return vfs.CommandDialectCmd }

func localCommandEnvironment(environment []string) []string {
	return commandEnvironmentWith(environment, applyCommandLiteralPercentEnv, "%")
}

func normalizeCommandOutput(line []byte) string {
	if utf8.Valid(line) {
		return string(line)
	}
	if encoding := vfs.GetSystemOEMEncoding(); encoding != nil {
		if decoded, err := encoding.NewDecoder().Bytes(line); err == nil {
			return strings.ToValidUTF8(string(decoded), "\uFFFD")
		}
	}
	return strings.ToValidUTF8(string(line), "\uFFFD")
}

func configureLocalProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func attachLocalProcessTree(cmd *exec.Cmd) localProcessTree {
	tree := &windowsLocalProcessTree{process: cmd.Process}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return tree
	}

	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		windows.CloseHandle(job)
		return tree
	}
	err = windows.AssignProcessToJobObject(job, process)
	windows.CloseHandle(process)
	if err != nil {
		windows.CloseHandle(job)
		return tree
	}
	tree.job = job
	return tree
}

type windowsLocalProcessTree struct {
	process *os.Process
	job     windows.Handle
}

func (p *windowsLocalProcessTree) Kill() error {
	if p.job != 0 {
		return windows.TerminateJobObject(p.job, 1)
	}
	// AssignProcessToJobObject can be denied by an enclosing legacy job. Keep
	// tree semantics in that case by asking the platform tool to walk the
	// parent/child snapshot, then fall back to the shell process itself.
	taskkill := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(p.process.Pid))
	if err := taskkill.Run(); err == nil {
		return nil
	}
	return p.process.Kill()
}

func (p *windowsLocalProcessTree) Close() error {
	if p.job == 0 {
		return nil
	}
	err := windows.CloseHandle(p.job)
	p.job = 0
	return err
}
