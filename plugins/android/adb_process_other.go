//go:build !windows

package androidfs

import "os/exec"

func configureADBCommand(command *exec.Cmd) {}
