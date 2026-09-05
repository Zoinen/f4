package fishplus

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanRun() {
		t.Skip("this host cannot run jobs")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	lines, code, err := c.RunOutput(ctx, dir, "ls")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("exit status = %d, want 0", code)
	}
	if len(lines) != 1 || lines[0] != "marker.txt" {
		t.Errorf("output = %v, want the one file in the directory", lines)
	}

	// The command runs where the files are, not where the session started.
	lines, _, err = c.RunOutput(ctx, dir, "pwd")
	if err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if len(lines) != 1 || !strings.HasSuffix(lines[0], filepath.Base(dir)) {
		t.Errorf("pwd said %v, want %q", lines, dir)
	}

	// A command that fails still ran, and what it printed is the answer.
	// Its exit status is reported rather than turned into an error.
	lines, code, err = c.RunOutput(ctx, dir, "echo 'a b  c'; exit 3")
	if err != nil {
		t.Fatalf("a failing command returned an error: %v", err)
	}
	if code != 3 {
		t.Errorf("exit status = %d, want 3", code)
	}
	if len(lines) != 1 || lines[0] != "a b  c" {
		t.Errorf("output = %v, want the spacing preserved", lines)
	}

	// Standard error arrives in the same stream, in order.
	lines, _, err = c.RunOutput(ctx, dir, "echo first; echo second >&2")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Errorf("output = %v, want both streams in order", lines)
	}

	// Polling counts complete lines. A command is allowed to exit without a
	// trailing newline; its final record still has to reach the line callback.
	lines, code, err = c.RunOutput(ctx, dir, "printf 'first\\ntail'")
	if err != nil {
		t.Fatalf("unterminated output: %v", err)
	}
	if code != 0 {
		t.Errorf("unterminated output exit status = %d, want 0", code)
	}
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "tail" {
		t.Errorf("unterminated output = %q, want [first tail]", lines)
	}

	if _, _, err := c.RunOutput(ctx, filepath.Join(dir, "no such dir"), "true"); err == nil {
		t.Error("a command in a directory that does not exist was accepted")
	}
	if _, err := c.StartCommand(ctx, dir, ""); err == nil {
		t.Error("an empty command was accepted")
	}

	// A command reading stdin must not take the request stream with it,
	// which is the whole reason this is a job.
	if _, _, err = c.RunOutput(ctx, dir, "cat"); err != nil {
		t.Fatalf("a command reading stdin: %v", err)
	}
	if err := c.Session().Noop(ctx); err != nil {
		t.Fatalf("session out of sync after running commands: %v", err)
	}
}
