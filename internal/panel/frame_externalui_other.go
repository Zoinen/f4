//go:build !windows && !android

package panel

import (
	"errors"
	"os/exec"
)

func AssociatedFileCommand(path string) (string, []string, bool) {
	if desktopAssociationCommand == "" {
		return "", nil, false
	}
	return desktopAssociationCommand, []string{path}, true
}

func openAssociatedPathOS(path, dir string) error {
	command, args, ok := AssociatedFileCommand(path)
	if !ok {
		return errors.New("opening items with a system application is unsupported on this platform")
	}
	err := runExternalUICommandOS(command, args, dir)
	if command == "xdg-open" && errors.Is(err, exec.ErrNotFound) {
		return runExternalUICommandOS("gio", []string{"open", path}, dir)
	}
	return err
}
