package multiarc

import (
	"context"
	"errors"
	"testing"
)

// errNotFoundStub is what a faked lookupTool returns for a binary the test
// wants to pretend is missing from PATH.
var errNotFoundStub = errors.New("multiarc test: not found")

// withFakeTools substitutes the archiver runner and lookupTool for the
// duration of a test and restores the production ones afterward, the same
// seam vfs/mount_linux.go's tests use for runExternalCommand/lookupExecutable
// (f4#415). run does not see the working directory; a test that needs it
// uses withFakeRunner.
func withFakeTools(t *testing.T, lookup func(string) (string, error), run func(ctx context.Context, name string, args ...string) ([]byte, []byte, error)) {
	t.Helper()
	var runIn func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)
	if run != nil {
		runIn = func(ctx context.Context, _ string, name string, args ...string) ([]byte, []byte, error) {
			return run(ctx, name, args...)
		}
	}
	withFakeRunner(t, lookup, runIn)
}

// withFakeRunner is withFakeTools with a runner that also sees the
// directory each command would have run in.
func withFakeRunner(t *testing.T, lookup func(string) (string, error), run func(ctx context.Context, dir, name string, args ...string) ([]byte, []byte, error)) {
	t.Helper()
	origLookup, origRun := lookupTool, runToolIn
	if lookup != nil {
		lookupTool = lookup
	}
	if run != nil {
		runToolIn = run
	}
	t.Cleanup(func() {
		lookupTool = origLookup
		runToolIn = origRun
	})
}

func TestToolAvailable(t *testing.T) {
	withFakeTools(t, func(name string) (string, error) {
		if name == "tar" {
			return "/usr/bin/tar", nil
		}
		return "", errors.New("not found")
	}, nil)

	if !toolAvailable("tar") {
		t.Error("expected tar to be reported available")
	}
	if toolAvailable("unzip") {
		t.Error("expected unzip to be reported unavailable")
	}
}
