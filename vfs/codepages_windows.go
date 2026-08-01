//go:build windows
// +build windows

package vfs

import (
	"fmt"
	"syscall"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
)

func getWindowsOEMCP() encoding.Encoding {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getOEMCP := kernel32.NewProc("GetOEMCP")
	if oemcp, _, _ := getOEMCP.Call(); oemcp != 0 {
		enc, err := htmlindex.Get(fmt.Sprintf("cp%d", oemcp))
		if err == nil {
			return enc
		}
	}
	return nil
}

func getWindowsACP() encoding.Encoding {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getACP := kernel32.NewProc("GetACP")
	if acp, _, _ := getACP.Call(); acp != 0 {
		enc, err := htmlindex.Get(fmt.Sprintf("windows-%d", acp))
		if err == nil {
			return enc
		}
	}
	return nil
}
