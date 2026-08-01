//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func checkAndDetach(attached bool) {
	if attached || os.Getenv("F4_DETACHED") == "1" {
		return
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "F4_DETACHED=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}

	null, _ := os.Open(os.DevNull)
	if null != nil {
		cmd.Stdin = null
		cmd.Stdout = null
		cmd.Stderr = null
	}

	if err := cmd.Start(); err == nil {
		os.Exit(0)
	}
}
