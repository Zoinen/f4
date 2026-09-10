//go:build !windows

package terminal

import (
	"context"
	"github.com/unxed/f4/vfs"
	"path/filepath"
	"testing"
)

func TestLocalCommandRunnerStreamsMergedLinesAndExitStatus(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	dir := t.TempDir()
	var got []string
	code, err := NewLocalCommandRunner().RunCommand(
		context.Background(),
		dir,
		"pwd; if IFS= read -r line; then exit 91; else printf 'stdin-eof\\n'; fi; printf 'stderr-line\\n' >&2; printf partial; exit 7",
		func(line string) { got = append(got, line) },
	)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	want := []string{dir, "stdin-eof", "stderr-line", "partial"}
	if len(got) != len(want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
	gotDir, gotDirErr := filepath.EvalSymlinks(got[0])
	wantDir, wantDirErr := filepath.EvalSymlinks(dir)
	if gotDirErr != nil || wantDirErr != nil || gotDir != wantDir {
		t.Fatalf("command directory = %q (%v), want %q (%v)", gotDir, gotDirErr, wantDir, wantDirErr)
	}
	for i := 1; i < len(want); i++ {
		if got[i] != want[i] {
			t.Fatalf("line %d = %q, want %q (all: %#v)", i, got[i], want[i], got)
		}
	}

	info := NewLocalCommandRunner().CommandRunnerInfo()
	if info.Dialect != vfs.CommandDialectPOSIX || info.MaxParallel != 0 {
		t.Fatalf("runner info = %+v", info)
	}
}
