//go:build windows

package proclist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCollectProcDetailsOnSelf reads the current test binary's own
// executable path -- always present, always readable by this same
// process -- the same "exercise the real syscall path on ourselves" idiom
// collector_windows_test.go already uses for collect() itself.
func TestCollectProcDetailsOnSelf(t *testing.T) {
	details := collectProcDetails(os.Getpid())

	if details.cmdline.err != nil {
		t.Fatalf("cmdline: %v", details.cmdline.err)
	}
	if len(details.cmdline.lines) != 1 || details.cmdline.lines[0] == "" {
		t.Fatalf("cmdline = %#v, want one non-empty executable path", details.cmdline.lines)
	}

	// Open handles stay unsupported (details_windows.go's own comment) and
	// must say so, not come back as a silently empty section.
	if details.openFiles.err != errDetailUnsupportedWindows {
		t.Fatalf("openFiles.err = %v, want errDetailUnsupportedWindows", details.openFiles.err)
	}
}

// The process reads its own PEB the way it would read another process': the
// command line and the environment must match what the runtime knows.
func TestCollectProcDetailsReadsOwnCommandLineAndEnvironment(t *testing.T) {
	t.Setenv("F4_PROCLIST_DETAILS_PROBE", "probe value")
	details := collectProcDetails(os.Getpid())

	if details.environ.err != nil {
		t.Skipf("environment not readable here: %v", details.environ.err)
	}
	found := false
	for _, line := range details.environ.lines {
		if line == "F4_PROCLIST_DETAILS_PROBE=probe value" {
			found = true
		}
	}
	if !found {
		t.Fatalf("environment lacks the variable this test set; got %d lines", len(details.environ.lines))
	}
	if len(details.cmdline.lines) != 1 || !strings.Contains(strings.ToLower(details.cmdline.lines[0]), strings.ToLower(filepath.Base(os.Args[0]))) {
		t.Fatalf("cmdline = %#v, want the test binary's command line", details.cmdline.lines)
	}
}

func TestReadWindowsExecutablePathOnANonexistentPidFails(t *testing.T) {
	section := readWindowsExecutablePath(1 << 30)
	if section.err == nil {
		t.Fatal("readWindowsExecutablePath on a made-up pid unexpectedly succeeded")
	}
}
