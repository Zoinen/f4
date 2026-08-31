//go:build !windows

package main

import "github.com/unxed/vtui"

func addWindowsLocationsDriveItem(*PanelsFrame, int, *vtui.VMenu) bool { return false }
