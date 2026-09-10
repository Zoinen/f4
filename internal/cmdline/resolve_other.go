//go:build !windows

package cmdline

// ResolveWindowsCommand is a no-op on non-Windows platforms.
func ResolveWindowsCommand(cmd string) string {
	return cmd
}

// IsBatchCommand is always false on non-Windows platforms.
func IsBatchCommand(cmd string) bool {
	return false
}
