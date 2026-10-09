package ap

// Options.Only - applying a chosen subset of a patch's modifications, the
// engine side of the review screen's "apply the checked edits" (f4#1606,
// docs/VTVIBE.md §7.3). Not ported from the Python reference, which has no
// such mode.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// onlyFixture writes files (name -> content) and the patch into a fresh
// temp dir and returns the dir and the patch path.
func onlyFixture(t *testing.T, files map[string]string, patch string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("setup %s: %v", name, err)
		}
	}
	patchPath := filepath.Join(t.TempDir(), "_case.ap")
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}
	return dir, patchPath
}

func readBack(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// TestOnlySubsetFromDryRun is the review screen's round trip: a dry run
// lists the modifications, the user unchecks one, and a real run with Only
// built from the dry run's Key()s applies exactly the rest.
func TestOnlySubsetFromDryRun(t *testing.T) {
	dir, patchPath := onlyFixture(t, map[string]string{"a.txt": "one\ntwo\nthree\n"},
		"bb000001 AP 3.2\n\nbb000001 FILE\na.txt\n\n"+
			"bb000001 REPLACE\nbb000001 snippet\none\nbb000001 content\nONE\n\n"+
			"bb000001 REPLACE\nbb000001 snippet\ntwo\nbb000001 content\nTWO\n\n"+
			"bb000001 REPLACE\nbb000001 snippet\nthree\nbb000001 content\nTHREE\n")

	dry := Apply(patchPath, dir, Options{DryRun: true, Silent: true})
	if dry.Status != StatusSuccess || len(dry.ModificationResults) != 3 {
		t.Fatalf("dry run = %s %+v, want SUCCESS with 3 results", dry.Status, dry.ModificationResults)
	}
	only := map[ModKey]bool{}
	for _, m := range dry.ModificationResults {
		if m.ModIdx != 1 {
			only[m.Key()] = true
		}
	}

	var buf bytes.Buffer
	res := Apply(patchPath, dir, Options{Only: only, Out: &buf})
	if res.Status != StatusSuccess {
		t.Fatalf("status = %s, want SUCCESS (out: %s)", res.Status, buf.String())
	}
	if got := readBack(t, dir, "a.txt"); got != "ONE\ntwo\nTHREE\n" {
		t.Fatalf("a.txt = %q, want mods #0 and #2 only", got)
	}
	if got := findModResult(t, res.ModificationResults, "a.txt", 1); got.Status != ModExcluded || got.Err != nil || got.Action != "REPLACE" || got.Locator != "two" {
		t.Fatalf("a.txt#1 = %+v, want {REPLACE, two, ModExcluded, nil}", got)
	}
	for _, i := range []int{0, 2} {
		if got := findModResult(t, res.ModificationResults, "a.txt", i); got.Status != ModOK {
			t.Fatalf("a.txt#%d = %+v, want ModOK", i, got)
		}
	}
	if !strings.Contains(buf.String(), "EXCLUDED") {
		t.Fatalf("progress output does not mention the excluded mod: %s", buf.String())
	}
	if pathExists(filepath.Join(dir, "afailed.ap")) {
		t.Fatal("afailed.ap written for a deselected (not failed) modification")
	}
}

// TestOnlyExcludedBlocks: a FILE block with nothing selected is left alone
// entirely - even one that would fail (missing file), a RENAME (ModIdx -1)
// and a whole-file DELETE.
func TestOnlyExcludedBlocks(t *testing.T) {
	dir, patchPath := onlyFixture(t, map[string]string{
		"a.txt": "x\n", "old.txt": "o\n", "gone.txt": "g\n",
	}, "bb000002 AP 3.2\n\n"+
		"bb000002 FILE\nmissing.txt\n\n"+
		"bb000002 REPLACE\nbb000002 snippet\nnope\nbb000002 content\nY\n\n"+
		"bb000002 FILE\nold.txt\n\nbb000002 RENAME\nnew.txt\n\n"+
		"bb000002 FILE\ngone.txt\n\nbb000002 DELETE\n\n"+
		"bb000002 FILE\na.txt\n\n"+
		"bb000002 REPLACE\nbb000002 snippet\nx\nbb000002 content\nX\n")

	res := Apply(patchPath, dir, Options{Silent: true, Only: map[ModKey]bool{{FilePath: "a.txt", ModIdx: 0}: true}})
	if res.Status != StatusSuccess {
		t.Fatalf("status = %s (%+v), want SUCCESS: an unselected missing file must not fail", res.Status, res.Error)
	}
	for _, k := range []ModKey{{"missing.txt", 0}, {"old.txt", -1}, {"gone.txt", 0}} {
		if got := findModResult(t, res.ModificationResults, k.FilePath, k.ModIdx); got.Status != ModExcluded || got.Err != nil {
			t.Fatalf("%v = %+v, want ModExcluded", k, got)
		}
	}
	if got := findModResult(t, res.ModificationResults, "old.txt", -1); got.Action != "RENAME" || got.Locator != "new.txt" {
		t.Fatalf("rename result = %+v, want RENAME -> new.txt", got)
	}
	if got := readBack(t, dir, "a.txt"); got != "X\n" {
		t.Fatalf("a.txt = %q, want the selected REPLACE applied", got)
	}
	if !pathExists(filepath.Join(dir, "old.txt")) || pathExists(filepath.Join(dir, "new.txt")) {
		t.Fatal("deselected RENAME was carried out")
	}
	if !pathExists(filepath.Join(dir, "gone.txt")) {
		t.Fatal("deselected whole-file DELETE was carried out")
	}
	if pathExists(filepath.Join(dir, "missing.txt")) || pathExists(filepath.Join(dir, "afailed.ap")) {
		t.Fatal("deselected block produced a file or an afailed.ap")
	}
}

// TestOnlyEmptySelection: a non-nil empty Only applies nothing (unlike nil,
// which applies everything).
func TestOnlyEmptySelection(t *testing.T) {
	dir, patchPath := onlyFixture(t, map[string]string{"a.txt": "x\n"},
		"bb000003 AP 3.2\n\nbb000003 FILE\na.txt\n\n"+
			"bb000003 REPLACE\nbb000003 snippet\nx\nbb000003 content\nX\n")

	res := Apply(patchPath, dir, Options{Silent: true, Only: map[ModKey]bool{}})
	if res.Status != StatusSuccess || len(res.ModificationResults) != 1 || res.ModificationResults[0].Status != ModExcluded {
		t.Fatalf("result = %s %+v, want SUCCESS with one ModExcluded", res.Status, res.ModificationResults)
	}
	if got := readBack(t, dir, "a.txt"); got != "x\n" {
		t.Fatalf("a.txt = %q, want unchanged", got)
	}
}

// Dependent modifications in one file: B's snippet ("three") exists only
// once A (two -> three) has run. B is listed first so that, with both
// selected, it takes the multi-pass retry to land.
const onlyDependentPatch = "bb000004 AP 3.2\n\nbb000004 FILE\na.txt\n\n" +
	"bb000004 REPLACE\nbb000004 snippet\nthree\nbb000004 content\nfour\n\n" + // B, #0
	"bb000004 REPLACE\nbb000004 snippet\ntwo\nbb000004 content\nthree\n" // A, #1

// TestOnlyDependentBothSelected: selecting both keeps the ordinary retry.
func TestOnlyDependentBothSelected(t *testing.T) {
	dir, patchPath := onlyFixture(t, map[string]string{"a.txt": "one\ntwo\n"}, onlyDependentPatch)
	res := Apply(patchPath, dir, Options{Silent: true, Only: map[ModKey]bool{{"a.txt", 0}: true, {"a.txt", 1}: true}})
	if res.Status != StatusSuccess {
		t.Fatalf("status = %s %+v, want SUCCESS", res.Status, res.ModificationResults)
	}
	if got := readBack(t, dir, "a.txt"); got != "one\nfour\n" {
		t.Fatalf("a.txt = %q, want both applied", got)
	}
}

// TestOnlyDependentWithoutPrerequisite: B selected without A is not pulled
// in or dropped silently - it fails with SNIPPET_NOT_FOUND like any
// modification whose snippet is not there, and goes to afailed.ap (A does
// not).
func TestOnlyDependentWithoutPrerequisite(t *testing.T) {
	dir, patchPath := onlyFixture(t, map[string]string{"a.txt": "one\ntwo\n"}, onlyDependentPatch)
	only := map[ModKey]bool{{"a.txt", 0}: true}

	res := Apply(patchPath, dir, Options{Silent: true, Only: only})
	if res.Status != StatusPartial {
		t.Fatalf("status = %s, want PARTIAL", res.Status)
	}
	b := findModResult(t, res.ModificationResults, "a.txt", 0)
	if b.Status != ModFailed || b.Err == nil || b.Err.Code != ErrSnippetNotFound {
		t.Fatalf("B = %+v, want ModFailed/SNIPPET_NOT_FOUND", b)
	}
	if a := findModResult(t, res.ModificationResults, "a.txt", 1); a.Status != ModExcluded {
		t.Fatalf("A = %+v, want ModExcluded", a)
	}
	if got := readBack(t, dir, "a.txt"); got != "one\ntwo\n" {
		t.Fatalf("a.txt = %q, want unchanged", got)
	}
	retry := readBack(t, dir, "afailed.ap")
	if !strings.Contains(retry, "four") || strings.Contains(retry, "two") {
		t.Fatalf("afailed.ap should hold B only:\n%s", retry)
	}

	// Strict mode: the same selection is a fatal error, nothing written.
	dir2, patchPath2 := onlyFixture(t, map[string]string{"a.txt": "one\ntwo\n"}, onlyDependentPatch)
	res = Apply(patchPath2, dir2, Options{Silent: true, Strict: true, Only: only})
	if res.Status != StatusFailed || res.Error == nil || res.Error.Code != ErrSnippetNotFound {
		t.Fatalf("strict = %s %+v, want FAILED/SNIPPET_NOT_FOUND", res.Status, res.Error)
	}
	if got := readBack(t, dir2, "a.txt"); got != "one\ntwo\n" {
		t.Fatalf("strict a.txt = %q, want unchanged", got)
	}
}

func TestModStatusExcludedString(t *testing.T) {
	if got := ModExcluded.String(); got != "EXCLUDED" {
		t.Fatalf("ModExcluded.String() = %q", got)
	}
}
