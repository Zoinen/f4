//go:build !windows

package main

import (
	"fmt"
	"runtime"
)

func readHostProductVersion(path string) (string, error) {
	return "", fmt.Errorf("pinned OpenConsole version check requires Windows (current OS: %s): %s", runtime.GOOS, path)
}

func runPinnedHost(path, reportPath string) error {
	_ = reportPath
	if _, err := verifyPinnedHost(path); err != nil {
		return err
	}
	return fmt.Errorf("pinned OpenConsole execution requires Windows")
}

func createServerHandle() error {
	return fmt.Errorf("pinned ConDrv server handle requires Windows")
}

func createClientHandle() error {
	return fmt.Errorf("pinned ConDrv client handle requires Windows")
}
