package multiarc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The unit tests in this package substitute runToolIn/lookupTool
// (withFakeTools, withFakeRunner), so they never exercise the production
// bodies of either -- the actual exec.CommandContext call and the actual
// exec.LookPath call. This file calls the real, unfaked ones directly, the
// same way the backends do at runtime (the *_realexec_test.go files next to
// it go further and drive whole archivers). It uses the
// "go" binary as its subject rather than a Unix tool such as sh: this
// package's tests run on the linux, macOS and windows CI cells alike (it
// carries no build constraint of its own), and "go" is the one binary
// every one of those cells is guaranteed to have on PATH.

func TestRunToolRealCommandSucceeds(t *testing.T) {
	stdout, _, err := runTool(context.Background(), "go", "version")
	if err != nil {
		t.Fatalf("runTool(go version): %v", err)
	}
	if !strings.Contains(string(stdout), "go version") {
		t.Errorf("stdout = %q, want it to contain %q", stdout, "go version")
	}
}

func TestRunToolRealCommandFails(t *testing.T) {
	_, stderr, err := runTool(context.Background(), "go", "f4-multiarc-not-a-real-subcommand")
	if err == nil {
		t.Fatal("expected an unknown go subcommand to exit non-zero")
	}
	if len(stderr) == 0 {
		t.Error("expected go to explain the unknown subcommand on stderr")
	}
}

// runToolIn's dir is the working directory the tool starts in: zip and 7z
// store a file under the relative path they were given, so a member staged
// as <stage>/dir/f.txt only lands as "dir/f.txt" when they run in <stage>.
// "go env GOMOD" names the go.mod governing the directory it runs in, which
// makes it a portable way to see that the directory really changed.
func TestRunToolInRealUsesWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module multiarcworkdir\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	stdout, stderr, err := runToolIn(context.Background(), dir, "go", "env", "GOMOD")
	if err != nil {
		t.Fatalf("runToolIn(go env GOMOD): %v (%s)", err, stderr)
	}
	got := strings.TrimSpace(string(stdout))
	want := filepath.Join(dir, "go.mod")
	gotInfo, gotErr := os.Stat(got)
	wantInfo, wantErr := os.Stat(want)
	if gotErr != nil || wantErr != nil || !os.SameFile(gotInfo, wantInfo) {
		t.Errorf("go env GOMOD in %s = %q, want %q", dir, got, want)
	}
}

func TestRunToolRealCommandNotFound(t *testing.T) {
	if _, _, err := runTool(context.Background(), "f4-multiarc-nonexistent-tool"); err == nil {
		t.Fatal("expected an error for a binary that does not exist")
	}
}

func TestLookupToolReal(t *testing.T) {
	if _, err := lookupTool("go"); err != nil {
		t.Errorf("lookupTool(go): %v", err)
	}
	if _, err := lookupTool("f4-multiarc-nonexistent-tool"); err == nil {
		t.Error("expected lookupTool to fail for a binary that does not exist")
	}
}

func TestToolAvailableReal(t *testing.T) {
	if !toolAvailable("go") {
		t.Error("expected go to be available through the real lookupTool")
	}
	if toolAvailable("f4-multiarc-nonexistent-tool") {
		t.Error("expected a nonexistent tool to be reported unavailable")
	}
}
