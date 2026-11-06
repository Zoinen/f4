package filegroups

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkingSetKeepsDeletionsAndDoesNotFollowFactoryUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "highlight.ini")
	defaults := []Group{
		{ID: "text", Fields: map[string]string{"Name": "Text", "Mask": "*.txt", "Icons.Enabled": "true"}},
		{ID: "images", Fields: map[string]string{"Name": "Images", "Mask": "*.png"}},
	}
	store := Store{Path: path, Defaults: defaults}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	working := tx.Groups()[:1]
	working[0].Fields["Name"] = "My text"
	working = append(working, Group{ID: "mine", Fields: map[string]string{"Mask": "*.mine"}})
	if err := tx.Commit(working); err != nil {
		t.Fatal(err)
	}
	if got := tx.Groups(); !reflect.DeepEqual(got, working) {
		t.Fatalf("Apply resurrected a deleted group: %v", got)
	}
	defaults[0].Fields["Mask"] = "*.factory-update"
	store.Defaults = append(defaults, Group{ID: "new-default", Fields: map[string]string{"Mask": "*"}})
	reloaded, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Groups(); !reflect.DeepEqual(got, working) {
		t.Fatalf("saved working set was re-layered over defaults: %v", got)
	}
	// Returning an editable draft must not expose the transaction's baseline.
	copy := reloaded.Groups()
	copy[0].Fields["Mask"] = "changed without Apply"
	if reloaded.Groups()[0].Fields["Mask"] != "*.txt" {
		t.Fatal("working set aliases the transaction")
	}
	if err := reloaded.Commit([]Group{}); err != nil {
		t.Fatal(err)
	}
	empty, err := store.Begin()
	if err != nil || len(empty.Groups()) != 0 {
		t.Fatalf("intentionally empty set restored defaults: %v, %v", empty, err)
	}
}

func TestWorkingSetUpgradePreservesUnknownDataAndFailedApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "highlight.ini")
	source := "# personal note\n[FileGroups]\nVersion=3\nVendor.Meta=keep\n" +
		"[Group.base]\nName=Edited\nVendor.Field=keep\n" +
		"[Unknown]\nWindows=C:\\Tools\\app.exe\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path, Defaults: []Group{{ID: "base", Fields: map[string]string{"Mask": "*.txt"}}}}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	working := tx.Groups()
	if working[0].Fields["Mask"] != "*.txt" {
		t.Fatal("legacy sparse file did not inherit its original base")
	}
	tx.store.Write = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	if err := tx.Commit([]Group{}); err == nil {
		t.Fatal("failed save reported success")
	}
	if !reflect.DeepEqual(tx.Groups(), working) {
		t.Fatal("failed save changed the baseline")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != source {
		t.Fatalf("failed save changed source: %v", err)
	}
	tx.store.Write = nil
	if err := tx.Commit(working); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".pre-filegroups-snapshot")
	if err != nil || string(backup) != source {
		t.Fatalf("snapshot conversion lost its exact sparse backup: %v", err)
	}
	for _, want := range []string{"Mode = snapshot", "# personal note", "Vendor.Meta = keep", "Vendor.Field = \"keep\"", "Windows=C:\\Tools\\app.exe"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved document missing %q", want)
		}
	}
}

func TestWorkingSetCommandsRemainIndependentOfStandardSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "highlight.ini")
	old := CommandSet{ID: "old", Name: "Standard application"}
	store := Store{Path: path, Defaults: []Group{{ID: "g", Fields: map[string]string{
		"Mask": "*.txt", "Associations.Enabled": "true",
	}, Commands: []CommandSet{old}}}}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	working := tx.Groups()
	app := CommandSet{ID: "editor", Name: "My editor",
		Commands: [6]string{"open !.!", "alt !.!", "view !.!", "altview !.!", "edit\nnext", `C:\Tools\edit.exe !.!`},
		Enabled:  [6]bool{true, false, true, false, true, true},
		Extra:    map[string]string{"Vendor.Command": "keep"},
	}
	working[0].Commands = []CommandSet{app}
	if err := tx.Commit(working); err != nil {
		t.Fatal(err)
	}
	store.Defaults[0].Commands = append(store.Defaults[0].Commands, CommandSet{ID: "new"})
	reloaded, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	commands := reloaded.Groups()[0].Commands
	if len(commands) != 1 || commands[0].ID != app.ID || commands[0].Commands != app.Commands ||
		commands[0].Enabled != app.Enabled || commands[0].Extra["Vendor.Command"] != "keep" {
		t.Fatalf("snapshot commands were patched against standard applications: %+v", commands)
	}
}

func TestWorkingSetRejectsUnknownOrIncompatibleMode(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{name: "unknown mode", source: "[FileGroups]\nVersion=3\nMode=future\n"},
		{name: "version two", source: "[FileGroups]\nVersion=2\nMode=snapshot\n"},
		{name: "legacy", source: "[FileGroups]\nMode=snapshot\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "highlight.ini")
			if err := os.WriteFile(path, []byte(tc.source), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := (Store{Path: path}).Begin(); err == nil {
				t.Fatal("unsupported working set was interpreted as a sparse file")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.source {
				t.Fatalf("invalid document overwritten: %v", err)
			}
		})
	}
}
