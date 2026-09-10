//go:build windows

package hostfs

import (
	"errors"
	"fmt"
	"os"
	"testing"

	winescape "github.com/unxed/libwinescape/go"
)

func TestPosixErrorMatchesHostFilesystemSentinels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		errno  winescape.Errno
		target error
	}{
		{name: "not-exist", errno: winescape.ENOENT, target: os.ErrNotExist},
		{name: "not-directory", errno: winescape.ENOTDIR, target: os.ErrNotExist},
		{name: "exists", errno: winescape.EEXIST, target: os.ErrExist},
		{name: "access", errno: winescape.EACCES, target: os.ErrPermission},
		{name: "permission", errno: winescape.EPERM, target: os.ErrPermission},
		{name: "invalid", errno: winescape.EINVAL, target: os.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := posixError{errno: tc.errno}
			if err.Error() != tc.errno.Error() {
				t.Fatalf("Error = %q, want %q", err.Error(), tc.errno.Error())
			}
			if !errors.Is(err, tc.errno) {
				t.Fatalf("errors.Is(error, errno) = false")
			}
			if !errors.Is(err, tc.target) {
				t.Fatalf("errors.Is(error, %v) = false", tc.target)
			}
			if errors.Is(err, errors.New("unrelated")) {
				t.Fatal("posixError matched an unrelated error")
			}
		})
	}

	wrapped := fmt.Errorf("wrapped errno: %w", winescape.ENOENT)
	if got := hostErr(wrapped); !errors.Is(got, os.ErrNotExist) {
		t.Fatalf("hostErr(wrapped errno) = %v, want fs.ErrNotExist match", got)
	}
}

func TestWineFileInfoModePreservesSpecialFileTypes(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode uint32
		want os.FileMode
	}{
		{name: "regular", mode: 0o100644, want: 0},
		{name: "character-device", mode: 0o20666, want: os.ModeDevice | os.ModeCharDevice},
		{name: "block-device", mode: 0o60660, want: os.ModeDevice},
		{name: "fifo", mode: 0o10644, want: os.ModeNamedPipe},
		{name: "socket", mode: 0o140755, want: os.ModeSocket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := wineStatToInfo("entry", "/tmp/entry", winescape.Stat_t{Mode: tc.mode}).Mode()
			if got&os.ModeType != tc.want&os.ModeType {
				t.Fatalf("Mode = %v, want type bits %v", got, tc.want)
			}
		})
	}
}
