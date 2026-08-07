//go:build !windows
// +build !windows

package vfs

import (
	"testing"

	"github.com/unxed/localecp"
)

func TestUnixSystemEncodings(t *testing.T) {
	oem := GetSystemOEMEncoding()
	ansi := GetSystemANSIEncoding()

	if oem != localecp.OEMEncoding {
		t.Errorf("expected OEM encoding %v, got %v", localecp.OEMEncoding, oem)
	}
	if ansi != localecp.ANSIEncoding {
		t.Errorf("expected ANSI encoding %v, got %v", localecp.ANSIEncoding, ansi)
	}
}
