//go:build !windows

package main

import "errors"

func updateDirNeedsElevation(string) bool { return false }

func isPermissionErrorForUpdate(error) bool { return false }

func runElevatedUpdate([]byte, string) error {
	return errors.New("UAC elevation is only available on Windows")
}

var runUpdateHelper = runUpdateHelperOS

func runUpdateHelperOS(string, string) error {
	return errors.New("update helper is only available on Windows")
}
