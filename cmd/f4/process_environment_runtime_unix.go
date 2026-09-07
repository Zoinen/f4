//go:build !windows

package main

import (
	"errors"
	"syscall"
)

func processEnvironmentProcessState(pid int) (alive bool, known bool) {
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return true, true
	case errors.Is(err, syscall.ESRCH):
		return false, true
	default:
		return false, false
	}
}
