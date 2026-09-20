//go:build windows

package dialog

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// aboutOSRows reports the Windows version the kernel itself answers with;
// RtlGetVersion is not subject to the manifest-based version lie.
func aboutOSRows() []aboutRow {
	v := windows.RtlGetVersion()
	return []aboutRow{
		{label: "Windows version", value: fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)},
	}
}
