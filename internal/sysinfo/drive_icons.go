package sysinfo

import (
	"strings"
)

const (
	DriveMenuIconOtherPanel = "panels-top-left"
	DriveMenuIconLocal      = "hard-drive"
	DriveMenuIconNetwork    = "network"
	DriveMenuIconPhysical   = "database"
	DriveMenuIconBookmark   = "folder"
	DriveMenuIconCloud      = "cloud"
	DriveMenuIconAndroid    = "android-logo"
	DriveMenuIconIOS        = "apple-logo"
	DriveMenuIconWindows    = "microsoft-windows-logo"
	DriveMenuIconAI         = "sparkles"
)

func RegisteredDriveIcon(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case lower == "android":
		return DriveMenuIconAndroid
	case lower == "ios":
		return DriveMenuIconIOS
	case lower == "ai":
		return DriveMenuIconAI
	case strings.Contains(lower, "net"):
		return DriveMenuIconNetwork
	case strings.Contains(lower, "cloud"):
		return DriveMenuIconCloud
	default:
		return DriveMenuIconPhysical
	}
}
