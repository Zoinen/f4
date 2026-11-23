//go:build !windows

package vfs

func NativeEntryName(_, fallback string) string {
	return fallback
}
