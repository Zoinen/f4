//go:build !windows

package vfs

import "os"

// readReparseTag: reparse points are a Windows concept; see the Windows file.
func readReparseTag(_ string, _ os.FileInfo) uint32 { return 0 }
