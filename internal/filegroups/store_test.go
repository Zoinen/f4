package filegroups

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpRefreshOnlyOnApply(t *testing.T) {
	const start = "# >>> f4 file groups help >>>"
	const end = "# <<< f4 file groups help <<<"
	for _, tc := range []struct {
		name, source string
	}{
		{
			name: "legacy",
			source: "# User note: keep this exactly\n[Highlight_101]\n" +
				"UseDefaults=1\nIncludeAttributes=Junction\nNormalFileName=foreground:#123456\n",
		},
		{
			name: "unified",
			source: "# User note: keep this exactly\n" + start + "\n# stale generated help\n" + end +
				"\n; another user note\n[FileGroups]\nVersion=2\nOrder=user\n" +
				"[Group.user]\nUseDefaults=1\nIncludeAttributes=Junction\nHighlight.Enabled=true\n" +
				"[Unknown.Section]\n# section note\nKeep=literal\\path\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "highlight.ini")
			if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			tx, err := (Store{Path: path}).Begin()
			if err != nil {
				t.Fatal(err)
			}
			read := func() string {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			if read() != tc.source {
				t.Fatal("loading refreshed comments before Apply")
			}
			if err := tx.Commit(tx.Groups()); err != nil {
				t.Fatal(err)
			}
			saved := read()
			for _, required := range []string{start, end, "# User note: keep this exactly", "UseDefaults", "Junction", "SelectedCursorColor"} {
				if !strings.Contains(saved, required) {
					t.Errorf("saved help or user data missing %q", required)
				}
			}
			if strings.Contains(saved, "stale generated help") {
				t.Error("generated help was not refreshed")
			}
			if strings.Count(saved, start) != 1 || strings.Count(saved, end) != 1 {
				t.Error("generated help block must appear exactly once")
			}
			if tc.name == "unified" && !strings.Contains(saved, "[Unknown.Section]\n# section note\nKeep=literal\\path\n") {
				t.Error("unknown section changed")
			}
			if err := tx.Commit(tx.Groups()); err != nil {
				t.Fatal(err)
			}
			if read() != saved {
				t.Error("repeated Apply changed an already current document")
			}
		})
	}
}

func TestHelpRefreshFailedApplyPreservesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "highlight.ini")
	source := []byte("# personal note\n[FileGroups]\nVersion=2\n")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path, Write: func(string, []byte, os.FileMode) error {
		return errors.New("disk full")
	}}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(tx.Groups()); err == nil {
		t.Fatal("failed Apply reported success")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(source) {
		t.Fatalf("failed Apply changed the on-disk help: %v", err)
	}
	tx.store.Write = nil
	if err := tx.Commit(tx.Groups()); err != nil {
		t.Fatalf("failed Apply advanced the baseline: %v", err)
	}
}

func TestMigrationWritesOnlyOnApply(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "highlight.ini")
	assoc := filepath.Join(dir, "associations.ini")
	legacy := []byte("# keep\n[Highlight_10]\nMask=*.png\nNormalFileName=foreground:#123456\nGroup=7\n[SortGroup_2]\nMask=*.zip\n")
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	commands := []byte("[Associations/Type0]\nMask=*.png\nDescription=Images\nExecute=C:\\Tools\\open.exe !.!\nState=3\nAltExec=alternate\n")
	if err := os.WriteFile(assoc, commands, 0600); err != nil {
		t.Fatal(err)
	}
	tx, err := (Store{Path: path, AssociationsPath: assoc}).Begin()
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(legacy) {
		t.Fatal("Begin changed disk")
	}
	groups := tx.Groups()
	if len(groups) != 3 || groups[0].Fields["MaskSyntax"] != "legacy-glob" || groups[0].Fields["NormalColor"] != "foreground:#123456" {
		t.Fatal(groups)
	}
	if !groups[2].Commands[0].Enabled[1] || groups[2].Commands[0].Commands[0] != `C:\Tools\open.exe !.!` {
		t.Fatal("command import changed")
	}
	if err := tx.Commit(groups); err != nil {
		t.Fatal(err)
	}
	backup, _ := os.ReadFile(path + ".pre-filegroups")
	if string(backup) != string(legacy) {
		t.Fatal("bad backup")
	}
	untouched, _ := os.ReadFile(assoc)
	if string(untouched) != string(commands) {
		t.Fatal("legacy associations changed")
	}
	reloaded, err := (Store{Path: path, AssociationsPath: assoc}).Begin()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Migrated() || len(reloaded.Groups()) != 3 {
		t.Fatal("migration not idempotent")
	}
}

func TestCommitConflictAndFailureAreRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "highlight.ini")
	store := Store{Path: path, Write: func(string, []byte, os.FileMode) error { return errors.New("disk full") }}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	groups := []Group{{ID: "x", Fields: map[string]string{"Name": "X"}}}
	if err := tx.Commit(groups); err == nil {
		t.Fatal("write failure ignored")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed Apply published")
	}
	tx.store.Write = nil
	if err := tx.Commit(groups); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# external edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(groups); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict=%v", err)
	}
}

func TestFactoryDifferencesAndTombstones(t *testing.T) {
	defaults := []Group{{ID: "a", Fields: map[string]string{"Name": "A", "NormalColor": "blue"}}, {ID: "b", Fields: map[string]string{"Name": "B"}}}
	effective := []Group{defaults[0].Clone()}
	effective[0].Fields["NormalColor"] = ""
	patch := Differences(defaults, effective)
	if _, exists := patch[0].Fields["Name"]; exists {
		t.Fatal("default frozen")
	}
	if patch[1].Fields["Enabled"] != "false" {
		t.Fatal("missing tombstone")
	}
	got := Merge(defaults, patch)
	if got[0].Fields["NormalColor"] != "" || got[1].Enabled("Highlight") {
		t.Fatal(got)
	}
}
