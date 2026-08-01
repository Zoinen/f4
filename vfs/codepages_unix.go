//go:build !windows
// +build !windows

package vfs

import "golang.org/x/text/encoding"

func getWindowsOEMCP() encoding.Encoding {
	return nil
}

func getWindowsACP() encoding.Encoding {
	return nil
}
