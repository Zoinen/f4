//go:build !windows

package update

import (
	"errors"
)

func dirNeedsElevation(string) bool { return false }

func isPermissionError(error) bool { return false }

func runElevated([]byte, string) error {
	return errors.New("UAC elevation is only available on Windows")
}

var RunHelper = runHelperOS

func runHelperOS(string, string) error {
	return errors.New("update helper is only available on Windows")
}
