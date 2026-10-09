package history

import (
	"runtime"
	"testing"
	"time"
)

func TestParseBashHistoryPlainFormat(t *testing.T) {
	fixture := "ls -la\ncd /tmp\n\ngit status\n"
	got := ParseBashHistory([]byte(fixture))
	want := []string{"ls -la", "cd /tmp", "git status"}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("record %d = %q, want %q", i, got[i].Name, name)
		}
		if !got[i].Timestamp.IsZero() {
			t.Errorf("record %d has timestamp %v, want zero (plain bash format has none)", i, got[i].Timestamp)
		}
	}
}

func TestParseBashHistoryWithTimestampComments(t *testing.T) {
	// HISTTIMEFORMAT-enabled bash writes a "#<epoch>" line before the
	// command it timestamps.
	fixture := "#1610000000\nls -la\ncd /tmp\n"
	got := ParseBashHistory([]byte(fixture))
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2: %+v", len(got), got)
	}
	if got[0].Name != "ls -la" {
		t.Errorf("record 0 name = %q, want %q", got[0].Name, "ls -la")
	}
	if want := time.Unix(1610000000, 0); !got[0].Timestamp.Equal(want) {
		t.Errorf("record 0 timestamp = %v, want %v", got[0].Timestamp, want)
	}
	if got[1].Name != "cd /tmp" {
		t.Errorf("record 1 name = %q, want %q", got[1].Name, "cd /tmp")
	}
	if !got[1].Timestamp.IsZero() {
		t.Errorf("record 1 has timestamp %v, want zero (no preceding comment)", got[1].Timestamp)
	}
}

func TestParseZshHistoryExtendedFormat(t *testing.T) {
	fixture := ": 1610000000:0;ls -la\n" +
		": 1610000005:2;git status\n" +
		"plain command, no metadata\n"
	got := ParseZshHistory([]byte(fixture))
	if len(got) != 3 {
		t.Fatalf("got %d records, want 3: %+v", len(got), got)
	}

	if got[0].Name != "ls -la" {
		t.Errorf("record 0 name = %q, want %q (metadata prefix must be stripped)", got[0].Name, "ls -la")
	}
	if want := time.Unix(1610000000, 0); !got[0].Timestamp.Equal(want) {
		t.Errorf("record 0 timestamp = %v, want %v", got[0].Timestamp, want)
	}

	if got[1].Name != "git status" {
		t.Errorf("record 1 name = %q, want %q", got[1].Name, "git status")
	}
	if want := time.Unix(1610000005, 0); !got[1].Timestamp.Equal(want) {
		t.Errorf("record 1 timestamp = %v, want %v", got[1].Timestamp, want)
	}

	if got[2].Name != "plain command, no metadata" {
		t.Errorf("record 2 name = %q, want unchanged plain line", got[2].Name)
	}
	if !got[2].Timestamp.IsZero() {
		t.Errorf("record 2 has timestamp %v, want zero (plain line has none)", got[2].Timestamp)
	}
}

func TestParseShellHistorySkipsMalformedAndBinaryLines(t *testing.T) {
	// A mix of good lines and real-world garbage: invalid UTF-8 (a lone
	// continuation byte), an embedded NUL, and a stray control byte, as can
	// happen from a mis-encoded paste landing in a history file. None of
	// these must crash the importer or contaminate a neighboring record;
	// each bad line is simply skipped.
	fixture := "echo good-1\n" +
		"echo bad-utf8-\xff-here\n" +
		"echo good-2\n" +
		"echo bad-nul-\x00-here\n" +
		"echo good-3\n" +
		"echo bad-control-\x01-here\n" +
		"echo good-4\n"

	got := ParseBashHistory([]byte(fixture))
	want := []string{"echo good-1", "echo good-2", "echo good-3", "echo good-4"}
	if len(got) != len(want) {
		t.Fatalf("ParseBashHistory: got %d records, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("ParseBashHistory record %d = %q, want %q", i, got[i].Name, name)
		}
	}

	// Same fixture through the zsh parser: it must survive it too, whether
	// or not the surviving lines carry extended metadata.
	gotZsh := ParseZshHistory([]byte(fixture))
	if len(gotZsh) != len(want) {
		t.Fatalf("ParseZshHistory: got %d records, want %d: %+v", len(gotZsh), len(want), gotZsh)
	}
	for i, name := range want {
		if gotZsh[i].Name != name {
			t.Errorf("ParseZshHistory record %d = %q, want %q", i, gotZsh[i].Name, name)
		}
	}
}

func TestMergeShellHistoryRecordsDeduplicates(t *testing.T) {
	current := []HistoryRecord{
		{Name: "git status"},
		{Name: "ls -la"},
	}
	imported := []HistoryRecord{
		{Name: "ls -la"},     // duplicate of current, must not be re-added
		{Name: "cd /tmp"},    // new
		{Name: "git status"}, // duplicate
		{Name: "cd /tmp"},    // duplicate of an entry within imported itself
		{Name: "make build"}, // new
	}

	merged, added := MergeShellHistoryRecords(current, imported, 0)
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	wantNames := []string{"git status", "ls -la", "cd /tmp", "make build"}
	if len(merged) != len(wantNames) {
		t.Fatalf("merged = %+v, want names %v", merged, wantNames)
	}
	for i, name := range wantNames {
		if merged[i].Name != name {
			t.Errorf("merged[%d].Name = %q, want %q", i, merged[i].Name, name)
		}
	}
}

func TestMergeShellHistoryRecordsRespectsLimit(t *testing.T) {
	current := []HistoryRecord{{Name: "a"}, {Name: "b"}}
	imported := []HistoryRecord{{Name: "c"}, {Name: "d"}, {Name: "e"}}

	merged, added := MergeShellHistoryRecords(current, imported, 3)
	if len(merged) != 3 {
		t.Fatalf("merged = %+v, want length 3", merged)
	}
	if added != 3 {
		// added counts everything newly appended before the cap is applied,
		// matching the far2l importer's existing "imported - len(current)"
		// convention of counting appended records rather than surviving ones.
		t.Fatalf("added = %d, want 3", added)
	}
}

func TestShellHistoryPathPrefersHistfile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell history import is POSIX-only (bash/zsh); the POSIX-style fixture paths below get backslashed by filepath.Join on Windows")
	}
	if got := ShellHistoryPath(ShellBash, "/custom/histfile", "/home/user"); got != "/custom/histfile" {
		t.Errorf("ShellHistoryPath with HISTFILE set = %q, want %q", got, "/custom/histfile")
	}
	if got := ShellHistoryPath(ShellBash, "", "/home/user"); got != "/home/user/.bash_history" {
		t.Errorf("ShellHistoryPath bash default = %q, want %q", got, "/home/user/.bash_history")
	}
	if got := ShellHistoryPath(ShellZsh, "", "/home/user"); got != "/home/user/.zsh_history" {
		t.Errorf("ShellHistoryPath zsh default = %q, want %q", got, "/home/user/.zsh_history")
	}
	if got := ShellHistoryPath(ShellBash, "   ", "/home/user"); got != "/home/user/.bash_history" {
		t.Errorf("ShellHistoryPath with blank HISTFILE = %q, want default", got)
	}
}
