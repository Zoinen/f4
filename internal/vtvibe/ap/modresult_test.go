package ap

// Result.ModificationResults is a per-modification report on top of the
// aggregate Status/FailedFiles Apply already returned before f4#1606's
// "part 4/N" step - see the ModificationResult doc comment in apply.go for
// why this exists (a future patch review screen, docs/VTVIBE.md §17). These
// tests are not ported from anywhere (the Python reference has no
// equivalent field), so they live in their own file rather than
// cases_test.go/run_tests_test.go, same reasoning as dryrun_test.go.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// findModResult returns the first ModificationResult matching file/modIdx,
// failing the test if there isn't one.
func findModResult(t *testing.T, results []ModificationResult, file string, modIdx int) ModificationResult {
	t.Helper()
	for _, r := range results {
		if r.FilePath == file && r.ModIdx == modIdx {
			return r
		}
	}
	t.Fatalf("no ModificationResult for %s#%d in %+v", file, modIdx, results)
	return ModificationResult{}
}

func TestModificationResultsOKAndSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("setup a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("setup b.txt: %v", err)
	}
	patchPath := filepath.Join(dir, "_case.ap")
	patch := "aa000001 AP 3.2\n\n" +
		"aa000001 FILE\na.txt\n\n" +
		"aa000001 REPLACE\naa000001 snippet\nline2\naa000001 content\nLINE2\n\n" +
		// b.txt: REPLACE whose snippet already reads as the requested
		// content - the idempotency skip path in modify.go
		// ("REPLACE content already present").
		"aa000001 FILE\nb.txt\n\n" +
		"aa000001 REPLACE\naa000001 snippet\nhello\naa000001 content\nhello\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}

	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{Out: &buf})
	if result.Status != StatusSuccess {
		t.Fatalf("status = %s, want %s (out: %s)", result.Status, StatusSuccess, buf.String())
	}
	if len(result.ModificationResults) != 2 {
		t.Fatalf("ModificationResults = %+v, want 2 entries", result.ModificationResults)
	}

	got := findModResult(t, result.ModificationResults, "a.txt", 0)
	if got.Status != ModOK || got.Action != "REPLACE" || got.Locator != "line2" || got.Err != nil {
		t.Fatalf("a.txt#0 = %+v, want {ModOK, REPLACE, %q, nil}", got, "line2")
	}

	got = findModResult(t, result.ModificationResults, "b.txt", 0)
	if got.Status != ModSkipped || got.Action != "REPLACE" || got.Locator != "hello" || got.Err != nil {
		t.Fatalf("b.txt#0 = %+v, want {ModSkipped, REPLACE, %q, nil}", got, "hello")
	}
}

func TestModificationResultsFailed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte("line1\n"), 0o600); err != nil {
		t.Fatalf("setup c.txt: %v", err)
	}
	patchPath := filepath.Join(dir, "_case.ap")
	patch := "aa000002 AP 3.2\n\naa000002 FILE\nc.txt\n\n" +
		"aa000002 REPLACE\naa000002 snippet\nnonexistent line\naa000002 content\nX\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}

	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{Out: &buf})
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want %s (out: %s)", result.Status, StatusPartial, buf.String())
	}
	if len(result.ModificationResults) != 1 {
		t.Fatalf("ModificationResults = %+v, want 1 entry", result.ModificationResults)
	}

	got := findModResult(t, result.ModificationResults, "c.txt", 0)
	if got.Status != ModFailed || got.Action != "REPLACE" || got.Locator != "nonexistent line" {
		t.Fatalf("c.txt#0 = %+v, want {ModFailed, REPLACE, %q, ...}", got, "nonexistent line")
	}
	if got.Err == nil || got.Err.Code != ErrSnippetNotFound {
		t.Fatalf("c.txt#0.Err = %+v, want Code %s", got.Err, ErrSnippetNotFound)
	}

	// The file was untouched: a failed modification must not show up as
	// applied on disk.
	b, err := os.ReadFile(filepath.Join(dir, "c.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(b) != "line1\n" {
		t.Fatalf("c.txt = %q, want unchanged", b)
	}
}

// TestModificationResultsPartialWithinFile covers the case the sub-item (a)
// of f4#1606's remaining "vtvibe integration" work explicitly calls out:
// one FILE block where some modifications succeed and others fail - the
// aggregate FailedFiles view (pre-existing) only says "c.txt had a
// failure", ModificationResults must say exactly which one(s).
func TestModificationResultsPartialWithinFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "d.txt"), []byte("line1\nline2\nline3\n"), 0o600); err != nil {
		t.Fatalf("setup d.txt: %v", err)
	}
	patchPath := filepath.Join(dir, "_case.ap")
	patch := "aa000003 AP 3.2\n\naa000003 FILE\nd.txt\n\n" +
		"aa000003 REPLACE\naa000003 snippet\nline1\naa000003 content\nLINE1\n\n" +
		"aa000003 REPLACE\naa000003 snippet\nnonexistent line\naa000003 content\nX\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}

	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{Out: &buf})
	if result.Status != StatusPartial {
		t.Fatalf("status = %s, want %s (out: %s)", result.Status, StatusPartial, buf.String())
	}
	if len(result.FailedFiles) != 1 || result.FailedFiles[0] != "d.txt" {
		t.Fatalf("FailedFiles = %v, want [d.txt]", result.FailedFiles)
	}
	if len(result.ModificationResults) != 2 {
		t.Fatalf("ModificationResults = %+v, want 2 entries", result.ModificationResults)
	}

	ok := findModResult(t, result.ModificationResults, "d.txt", 0)
	if ok.Status != ModOK || ok.Locator != "line1" {
		t.Fatalf("d.txt#0 = %+v, want {ModOK, ..., line1, ...}", ok)
	}
	failed := findModResult(t, result.ModificationResults, "d.txt", 1)
	if failed.Status != ModFailed || failed.Err == nil || failed.Err.Code != ErrSnippetNotFound {
		t.Fatalf("d.txt#1 = %+v, want ModFailed/SNIPPET_NOT_FOUND", failed)
	}

	// The successful first modification was still written to disk despite
	// the second one failing - tolerant mode applies what it can.
	b, err := os.ReadFile(filepath.Join(dir, "d.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(b) != "LINE1\nline2\nline3\n" {
		t.Fatalf("d.txt = %q, want the first REPLACE applied", b)
	}
}

func TestModificationResultsRename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "old.txt"), []byte("x\n"), 0o600); err != nil {
		t.Fatalf("setup old.txt: %v", err)
	}
	patchPath := filepath.Join(dir, "_case.ap")
	patch := "aa000004 AP 3.2\n\naa000004 FILE\nold.txt\n\naa000004 RENAME\nnew.txt\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}

	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{Out: &buf})
	if result.Status != StatusSuccess {
		t.Fatalf("status = %s, want %s (out: %s)", result.Status, StatusSuccess, buf.String())
	}
	if len(result.ModificationResults) != 1 {
		t.Fatalf("ModificationResults = %+v, want 1 entry", result.ModificationResults)
	}
	got := result.ModificationResults[0]
	if got.FilePath != "old.txt" || got.ModIdx != -1 || got.Action != "RENAME" || got.Locator != "new.txt" || got.Status != ModOK {
		t.Fatalf("rename result = %+v, want {old.txt, -1, RENAME, new.txt, ModOK}", got)
	}
}
