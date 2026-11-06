//go:build !darwin

package panel

import "github.com/unxed/vtinput"

func isFileContextMenuClick(*vtinput.InputEvent) bool { return false }
