//go:build tty_only

package gui

import "fmt"

const Available = false

func BackendBuilt(string) bool {
	return false
}

func RunGui(string, func()) error {
	return fmt.Errorf("GUI mode is unavailable in the TTY-only build")
}
