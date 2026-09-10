//go:build !windows

package panel

func driveMenuPlatformKind(path string) driveMenuKind {
	if path == "" {
		return driveMenuKindUnknown
	}
	return driveMenuKindFixed
}
