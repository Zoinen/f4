package gui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var processWorktreeBranch = sync.OnceValue(resolveProcessWorktreeBranch)

func CurrentWorktreeBranchName() string {
	return processWorktreeBranch()
}

func resolveProcessWorktreeBranch() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}

	command := exec.Command(
		"git", "-C", filepath.Dir(executable), "--no-optional-locks",
		"branch", "--show-current")
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
