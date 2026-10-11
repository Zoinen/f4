package ap

// DryRun is not exercised by any of the ported cases.py cases (the Python
// reference tests it separately, via its own CLI --dry-run flag, outside
// run_tests.py/cases.py), so it gets its own small suite here rather than a
// 46th "case".

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyDryRunLeavesFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "a.txt")
	original := "line1\nline2\nline3\n"
	if err := os.WriteFile(target, []byte(original), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	patchPath := filepath.Join(dir, "_case.ap")
	patch := "aa000001 AP 3.2\n\naa000001 FILE\na.txt\n\n" +
		"aa000001 REPLACE\naa000001 snippet\nline2\naa000001 content\nLINE2\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{DryRun: true, Out: &buf})
	if result.Status != StatusSuccess {
		t.Fatalf("status = %s, want %s", result.Status, StatusSuccess)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != original {
		t.Fatalf("dry run modified the file: got %q, want unchanged %q", got, original)
	}

	if out := buf.String(); !strings.Contains(out, "write a.txt") {
		t.Fatalf("dry run output = %q, want it to mention the planned write", out)
	}
}

func TestApplyDryRunReportsRenameAndDelete(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gone.txt"), []byte("y\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	patchPath := filepath.Join(dir, "_case.ap")
	patch := "aa000002 AP 3.2\n\n" +
		"aa000002 FILE\nold.txt\n\naa000002 RENAME\nnew.txt\n\n" +
		"aa000002 FILE\ngone.txt\n\naa000002 DELETE\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{DryRun: true, Out: &buf})
	if result.Status != StatusSuccess {
		t.Fatalf("status = %s, want %s", result.Status, StatusSuccess)
	}

	for _, rel := range []string{"old.txt", "gone.txt"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("dry run touched %q on disk: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); err == nil {
		t.Fatal("dry run created new.txt on disk")
	}

	out := buf.String()
	if !strings.Contains(out, "rename old.txt -> new.txt") {
		t.Fatalf("dry run output = %q, want it to mention the planned rename", out)
	}
	if !strings.Contains(out, "delete gone.txt") {
		t.Fatalf("dry run output = %q, want it to mention the planned delete", out)
	}
}

func TestApplyDryRunStillReportsFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("line1\n"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	patchPath := filepath.Join(dir, "_case.ap")
	patch := "aa000003 AP 3.2\n\naa000003 FILE\na.txt\n\n" +
		"aa000003 REPLACE\naa000003 snippet\nno such line\naa000003 content\nX\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{DryRun: true, Out: &buf})
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want %s", result.Status, StatusPartial)
	}

	// afailed.ap/afailed.md are validation output, written before the
	// commit/dry-run split, so a dry run must still produce them - the
	// reference does the same (dry_run only guards the OS write phase).
	if _, err := os.Stat(filepath.Join(dir, "afailed.ap")); err != nil {
		t.Fatalf("afailed.ap was not written on a dry run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "afailed.md")); err != nil {
		t.Fatalf("afailed.md was not written on a dry run: %v", err)
	}
}
