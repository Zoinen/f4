package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExecutablePath is the path of the real f4 binary, symlinks followed, and
// ExecutableDir its folder: where the files shipped beside it (help/*.hlf) are,
// however f4 was started.
//
// os.Args[0] is not that. It is the bare "f4" when the shell found the program
// on $PATH, so its directory was "." and help was looked for in the current
// directory -- found only when f4 happened to be started from its own folder
// (f4#1976). It is the link's name when f4 was started through a symlink, so
// help was looked for beside the link instead of beside the binary. On the
// universal Linux build it is an in-memory image. This resolves all of them:
// the wired Executable first (it knows about the universal build), then
// os.Args[0] looked up on $PATH, then the symlinks followed.
func ExecutablePath() string {
	exe, err := Executable()
	if err != nil || exe == "" {
		exe = os.Args[0]
		if !strings.ContainsRune(exe, filepath.Separator) {
			if found, err := exec.LookPath(exe); err == nil {
				exe = found
			}
		}
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return exe
}

// ExecutableDir is the folder of ExecutablePath.
func ExecutableDir() string {
	return filepath.Dir(ExecutablePath())
}
