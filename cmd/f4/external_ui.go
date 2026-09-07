package main

import (
	"os/exec"
	"runtime"
)

// externalUICommandRunner is the process boundary for commands that hand
// control to the desktop environment. Keeping this separate from ordinary
// command execution lets the test binary disable native UI globally.
type externalUICommandRunner func(command string, args []string, dir string) error

var defaultExternalUICommandRunner externalUICommandRunner = runExternalUICommandOS

func runExternalUICommandOS(command string, args []string, dir string) error {
	cmd := exec.Command(command, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.Run()
}

func (pf *PanelsFrame) runExternalUICommand(command string, args []string, dir string) error {
	runner := defaultExternalUICommandRunner
	if pf != nil && pf.externalUIRunner != nil {
		runner = pf.externalUIRunner
	}
	if runner == nil {
		return nil
	}
	return runner(command, append([]string(nil), args...), dir)
}

func systemFileManagerCommand(path string, isDir bool) (string, []string, bool) {
	switch runtime.GOOS {
	case "linux":
		return "xdg-open", []string{path}, true
	case "windows":
		if isDir {
			return "explorer.exe", []string{path}, true
		}
		return "explorer.exe", []string{"/select,", path}, true
	case "darwin":
		return "open", []string{path}, true
	default:
		return "", nil, false
	}
}

func associatedFileCommand(path string) (string, []string, bool) {
	switch runtime.GOOS {
	case "linux":
		return "xdg-open", []string{path}, true
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", path}, true
	case "darwin":
		return "open", []string{path}, true
	default:
		return "", nil, false
	}
}
