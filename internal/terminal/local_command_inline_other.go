//go:build !windows

package terminal

import (
	"os"
	"os/exec"
)

// RunLocalCommandInline runs one command with the platform's normal shell and
// inherited stdio. Windows/Wine has a separate implementation because a
// Windows Go process cannot reach the host ELF shell through os/exec.
func RunLocalCommandInline(dir, command string) error {
	cmd := exec.Command(GetSystemShell(), "-c", command) // #nosec G204 -- this is the intentional local shell command line.
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run()
}
