//go:build !windows

package panel

import (
	"github.com/unxed/vtui"
)

func addWindowsLocationsDriveItem(*PanelsFrame, int, *vtui.VMenu) bool { return false }
