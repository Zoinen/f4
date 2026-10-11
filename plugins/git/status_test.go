package git

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestParseStatusCleanRepoOnMain(t *testing.T) {
	output := "# branch.oid abcdef0123456789\n" +
		"# branch.head main\n" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +0 -0\n"

	got := parseStatus([]byte(output))
	want := statusResult{Branch: "main", HasCommit: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseStatus(clean) = %+v, want %+v", got, want)
	}
}

func TestParseStatusDetachedHead(t *testing.T) {
	got := parseStatus([]byte("# branch.oid abcdef0123456789\n# branch.head (detached)\n"))
	if !got.Detached || got.Branch != "" {
		t.Fatalf("parseStatus(detached) = %+v, want Detached=true, Branch=\"\"", got)
	}
}

func TestParseStatusOrdinaryChanges(t *testing.T) {
	// One staged-modified file, one unstaged-modified, one added, and one
	// untracked -- the four kinds `git status -s` shows most often.
	output := "# branch.head main\n" +
		"1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb internal/app/foo.go\n" +
		"1 .M N... 100644 100644 100644 ccccccc ccccccc README.md\n" +
		"1 A. N... 000000 100644 100644 0000000 ddddddd new_file.txt\n" +
		"? untracked.txt\n"

	got := parseStatus([]byte(output))
	want := []statusEntry{
		{XY: "M.", Path: "internal/app/foo.go"},
		{XY: ".M", Path: "README.md"},
		{XY: "A.", Path: "new_file.txt"},
		{XY: "??", Path: "untracked.txt"},
	}
	if !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("parseStatus(changes).Entries = %+v, want %+v", got.Entries, want)
	}
}

func TestParseStatusRename(t *testing.T) {
	output := "# branch.head main\n" +
		"2 R. N... 100644 100644 100644 aaaaaaa aaaaaaa R100 new_name.go\told_name.go\n"

	got := parseStatus([]byte(output))
	want := []statusEntry{
		{XY: "R.", Path: "new_name.go", OrigPath: "old_name.go"},
	}
	if !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("parseStatus(rename).Entries = %+v, want %+v", got.Entries, want)
	}
}

func TestParseStatusUnmergedConflict(t *testing.T) {
	output := "# branch.head main\n" +
		"u UU N... 100644 100644 100644 100644 aaaaaaa bbbbbbb ccccccc conflicted.txt\n"

	got := parseStatus([]byte(output))
	want := []statusEntry{{XY: "UU", Path: "conflicted.txt"}}
	if !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("parseStatus(unmerged).Entries = %+v, want %+v", got.Entries, want)
	}
}

func TestParseStatusPathWithSpaces(t *testing.T) {
	output := "# branch.head main\n" +
		"? my file with spaces.txt\n"

	got := parseStatus([]byte(output))
	want := []statusEntry{{XY: "??", Path: "my file with spaces.txt"}}
	if !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("parseStatus(spaces).Entries = %+v, want %+v", got.Entries, want)
	}
}

func TestParseStatusEmptyAndMalformedLinesAreSkipped(t *testing.T) {
	output := "# branch.head main\n\n1 M.\n"
	got := parseStatus([]byte(output))
	if len(got.Entries) != 0 {
		t.Fatalf("parseStatus(malformed) = %+v, want no entries", got)
	}
}

func TestSplitNFields(t *testing.T) {
	tests := []struct {
		line string
		n    int
		want []string
	}{
		{"1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb path with spaces", 8,
			[]string{"1", "M.", "N...", "100644", "100644", "100644", "aaaaaaa", "bbbbbbb", "path with spaces"}},
		{"too short", 5, nil},
		{"a b", 1, []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := splitNFields(tt.line, tt.n)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("splitNFields(%q, %d) = %v, want %v", tt.line, tt.n, got, tt.want)
		}
	}
}

func TestAvailable(t *testing.T) {
	original := lookupGit
	defer func() { lookupGit = original }()

	lookupGit = func(string) (string, error) { return "/usr/bin/git", nil }
	if !Available() {
		t.Error("Available() = false, want true when lookupGit succeeds")
	}

	lookupGit = func(string) (string, error) { return "", errors.New("not found") }
	if Available() {
		t.Error("Available() = true, want false when lookupGit fails")
	}
}

func TestRunGitInPrependsQuotepathAndPassesDir(t *testing.T) {
	original := execGit
	defer func() { execGit = original }()

	var gotDir string
	var gotArgs []string
	execGit = func(_ context.Context, dir string, args []string) ([]byte, error) {
		gotDir = dir
		gotArgs = args
		return []byte("ok"), nil
	}

	out, err := runGitIn(context.Background(), "/tmp/repo", "status", "--porcelain=v2")
	if err != nil {
		t.Fatalf("runGitIn returned error: %v", err)
	}
	if string(out) != "ok" {
		t.Fatalf("runGitIn output = %q, want %q", out, "ok")
	}
	if gotDir != "/tmp/repo" {
		t.Fatalf("dir = %q, want %q", gotDir, "/tmp/repo")
	}
	want := []string{"-c", "core.quotepath=false", "status", "--porcelain=v2"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("args = %v, want %v", gotArgs, want)
	}
}
