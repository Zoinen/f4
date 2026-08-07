//go:build !windows
// +build !windows

package vfs

import (
	"golang.org/x/text/encoding"

	"github.com/unxed/localecp"
)

func getWindowsOEMCP() encoding.Encoding {
	return localecp.OEMEncoding
}

func getWindowsACP() encoding.Encoding {
	return localecp.ANSIEncoding
}
