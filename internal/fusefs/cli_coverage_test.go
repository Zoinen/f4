package fusefs

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// runMount's own validation gate (as opposed to mountAndServe, which is
// exercised through withFakeBackend elsewhere in this package) was not
// covered: a request with no source must be refused before anything tries
// to touch a backend at all.
func TestRunMountRejectsInvalidSpec(t *testing.T) {
	spec := NewSpec()
	var out, errOut bytes.Buffer
	if code := runMount(spec, &out, &errOut); code != ExitUsage {
		t.Fatalf("exit code = %d, want ExitUsage; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "no mount source") {
		t.Fatalf("stderr = %q, want the validation error surfaced", errOut.String())
	}
}

// A mount that ends on its own -- the backend unmounted itself, or the
// source ran out -- must not be unmounted a second time.
func TestWaitOrSignalReturnsWhenWaitCompletes(t *testing.T) {
	oldUnmount := unmountOwn
	var unmountCalled atomic.Bool
	unmountOwn = func(string) error {
		unmountCalled.Store(true)
		return nil
	}
	t.Cleanup(func() { unmountOwn = oldUnmount })

	wait := func() {} // returns immediately, as if the mount had already ended

	finished := make(chan struct{})
	go func() {
		waitOrSignal(wait, "/mnt/does-not-matter")
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("waitOrSignal did not return once wait() completed")
	}
	if unmountCalled.Load() {
		t.Fatal("a mount that ended by itself must not be unmounted again")
	}
}

// TestWaitOrSignalUnmountsOnSIGTERM lives in cli_coverage_signal_test.go:
// syscall.Kill, which it uses to deliver a real SIGTERM to this process, does
// not exist on windows (nor on plan9 or js), so that file carries the same
// build tag as platform_unix.go to keep this one buildable everywhere.

// runUmount has two ways to bring a mount down: through this process's own
// unmountOwn when the record says this process owns it, and through
// systemUnmount otherwise. Only the second is covered by
// TestUmountUnknownTarget; this pins the first.
func TestRunUmountUsesUnmountOwnForItsOwnMount(t *testing.T) {
	useTempRegistry(t)
	mp := filepath.Join(t.TempDir(), "mnt")
	rec, err := Register(Record{Source: "a.tar", MountPoint: mp})
	if err != nil {
		t.Fatal(err)
	}

	oldUnmount := unmountOwn
	var calledWith string
	unmountOwn = func(mountPoint string) error {
		calledWith = mountPoint
		return nil
	}
	t.Cleanup(func() { unmountOwn = oldUnmount })

	spec := NewSpec()
	spec.Source = rec.MountPoint
	var out, errOut bytes.Buffer
	if code := runUmount(spec, &out, &errOut); code != ExitOK {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if calledWith != rec.MountPoint {
		t.Fatalf("unmountOwn called with %q, want %q", calledWith, rec.MountPoint)
	}
	if _, found := FindMount(rec.MountPoint); found {
		t.Fatal("a successful unmount must deregister the record")
	}
}

// TestListMountsJSONIsAlwaysAnArray only exercises the empty, --json path.
// The default, human-readable table -- header plus one row per mount -- had
// no test at all.
func TestRunListPrintsHumanReadableTable(t *testing.T) {
	useTempRegistry(t)
	mp := filepath.Join(t.TempDir(), "mnt")
	rec, err := Register(Record{Source: "a.tar", MountPoint: mp, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}

	spec := NewSpec()
	var out, errOut bytes.Buffer
	if code := runList(spec, &out, &errOut); code != ExitOK {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	got := out.String()
	if !strings.Contains(got, "MOUNT POINT") {
		t.Fatalf("the header is missing: %q", got)
	}
	if !strings.Contains(got, rec.MountPoint) || !strings.Contains(got, "a.tar") {
		t.Fatalf("the mount row is missing: %q", got)
	}
}

func TestRunListEmptyTableHasNoOutput(t *testing.T) {
	useTempRegistry(t)
	spec := NewSpec()
	var out, errOut bytes.Buffer
	if code := runList(spec, &out, &errOut); code != ExitOK {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if got := out.String(); got != "" {
		t.Fatalf("an empty mount list printed %q, want nothing", got)
	}
}

// TestIsBusy already pins the string-matching fallback (a fusermount
// message with no errno behind it); the errors.Is(err, syscall.EBUSY)
// branch, for an error that actually carries the errno, had nothing.
func TestIsBusyRecognisesWrappedEBUSY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("EBUSY is a POSIX errno")
	}
	err := &os.PathError{Op: "unmount", Path: "/mnt/x", Err: syscall.EBUSY}
	if !isBusy(err) {
		t.Fatal("a wrapped EBUSY must be recognised without string-matching")
	}
}
