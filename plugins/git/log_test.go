package git

import (
	"reflect"
	"testing"
)

func TestParseLog(t *testing.T) {
	output := []byte(
		"aaaa1111full" + logFieldSep + "aaaa111" + logFieldSep + "Alice" + logFieldSep + "2026-09-28" + logFieldSep + "First commit\n" +
			"bbbb2222full" + logFieldSep + "bbbb222" + logFieldSep + "Bob" + logFieldSep + "2026-09-27" + logFieldSep + "Second commit",
	)
	got := parseLog(output)
	want := []logEntry{
		{Hash: "aaaa1111full", ShortHash: "aaaa111", Author: "Alice", Date: "2026-09-28", Subject: "First commit"},
		{Hash: "bbbb2222full", ShortHash: "bbbb222", Author: "Bob", Date: "2026-09-27", Subject: "Second commit"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLog = %#v, want %#v", got, want)
	}
}

func TestParseLogSubjectMayContainTabsAndSpaces(t *testing.T) {
	got := parseLog([]byte("aaaa" + logFieldSep + "a" + logFieldSep + "Alice" + logFieldSep + "2026-09-28" + logFieldSep + "fix:\ttabs and  spaces"))
	if len(got) != 1 || got[0].Subject != "fix:\ttabs and  spaces" {
		t.Fatalf("parseLog with a tab/space-bearing subject = %#v", got)
	}
}

func TestParseLogSkipsMalformedLines(t *testing.T) {
	got := parseLog([]byte("not enough fields\n\naaaa" + logFieldSep + "a" + logFieldSep + "Alice" + logFieldSep + "2026-09-28" + logFieldSep + "ok"))
	if len(got) != 1 || got[0].Subject != "ok" {
		t.Fatalf("parseLog with malformed lines = %#v, want exactly the one well-formed entry", got)
	}
}

func TestParseLogEmpty(t *testing.T) {
	if got := parseLog([]byte("")); got != nil {
		t.Fatalf("parseLog(empty) = %#v, want nil", got)
	}
}

func TestParseNameStatusPlainAddDeleteAndRename(t *testing.T) {
	output := []byte("M\tfoo.go\nA\tbar.go\nD\tgone.go\nR100\told.go\tnew.go\nC087\tsrc.go\tcopy.go\n")
	got := parseNameStatus(output)
	want := []logDiffEntry{
		{Status: 'M', Path: "foo.go"},
		{Status: 'A', Path: "bar.go"},
		{Status: 'D', Path: "gone.go"},
		{Status: 'R', OrigPath: "old.go", Path: "new.go"},
		{Status: 'C', OrigPath: "src.go", Path: "copy.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNameStatus = %#v, want %#v", got, want)
	}
}

// TestParseNameStatusEmptyForAMergeCommit exercises the shape `git show
// --name-status` gives a merge commit (no -m/-c flag): no lines at all,
// not an error -- commitChangedFiles (logdiff.go) relies on this.
func TestParseNameStatusEmptyForAMergeCommit(t *testing.T) {
	if got := parseNameStatus([]byte("")); got != nil {
		t.Fatalf("parseNameStatus(empty) = %#v, want nil", got)
	}
}

func TestParseNameStatusSkipsBlankLines(t *testing.T) {
	got := parseNameStatus([]byte("\nM\tfoo.go\n\n"))
	if want := []logDiffEntry{{Status: 'M', Path: "foo.go"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNameStatus = %#v, want %#v", got, want)
	}
}

func TestLogDiffEntryOldPath(t *testing.T) {
	if got := (logDiffEntry{Path: "foo.go"}).oldPath(); got != "foo.go" {
		t.Errorf("oldPath(plain) = %q, want %q", got, "foo.go")
	}
	if got := (logDiffEntry{Path: "new.go", OrigPath: "old.go"}).oldPath(); got != "old.go" {
		t.Errorf("oldPath(rename) = %q, want %q", got, "old.go")
	}
}
