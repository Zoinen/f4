//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package dialog

import (
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// aboutOSRows reports the distribution and the kernel, far2l's os-release
// PRETTY_NAME and uname lines.
func aboutOSRows() []aboutRow {
	return []aboutRow{
		{label: "os-release PRETTY_NAME", value: osReleasePrettyName()},
		{label: "uname", value: unameString()},
	}
}

func osReleasePrettyName() string {
	// The freedesktop specification reads /etc/os-release and falls back
	// to /usr/lib/os-release.
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		file, err := os.Open(path) // #nosec G304 -- two fixed system paths
		if err != nil {
			continue
		}
		name := parseOSReleasePrettyName(file)
		_ = file.Close() // read-only; nothing to lose
		if name != "" {
			return name
		}
	}
	return ""
}

func unameString() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return ""
	}
	return strings.Join([]string{
		unix.ByteSliceToString(u.Sysname[:]),
		unix.ByteSliceToString(u.Release[:]),
		unix.ByteSliceToString(u.Version[:]),
		unix.ByteSliceToString(u.Machine[:]),
	}, " ")
}
