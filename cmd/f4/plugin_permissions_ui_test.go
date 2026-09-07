package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

// TestPermissionStoreGrantsAreOrdered pins the order, because Go randomises
// map iteration and a list that reshuffles between openings is one nobody can
// revoke from with any confidence.
func TestPermissionStoreGrantsAreOrdered(t *testing.T) {
	store := LoadPermissionStore(filepath.Join(t.TempDir(), "perms.json"))
	for _, grant := range []PermissionGrant{
		{Plugin: "notes", Permission: PermissionUnsafeStdlib, Decision: PermissionAllow},
		{Plugin: "archive", Permission: PermissionFFI, Decision: PermissionAllow},
		{Plugin: "notes", Permission: PermissionFFI, Decision: PermissionAllow},
	} {
		if err := store.Remember(grant.Plugin, grant.Permission, grant.Decision); err != nil {
			t.Fatalf("Remember: %v", err)
		}
	}

	want := []PermissionGrant{
		{Plugin: "archive", Permission: PermissionFFI, Decision: PermissionAllow},
		{Plugin: "notes", Permission: PermissionFFI, Decision: PermissionAllow},
		{Plugin: "notes", Permission: PermissionUnsafeStdlib, Decision: PermissionAllow},
	}
	got := store.Grants()
	if len(got) != len(want) {
		t.Fatalf("Grants() returned %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestRevokeDropsOneGrantAndKeepsTheRest separates Revoke from Forget: one is
// a user changing their mind about a line, the other is a plugin going away.
func TestRevokeDropsOneGrantAndKeepsTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perms.json")
	store := LoadPermissionStore(path)
	if err := store.Remember("notes", PermissionFFI, PermissionAllow); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	if err := store.Remember("notes", PermissionUnsafeStdlib, PermissionAllow); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	if err := store.Revoke("notes", PermissionFFI); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, ok := store.Decision("notes", PermissionFFI); ok {
		t.Error("the revoked permission is still granted")
	}
	if _, ok := store.Decision("notes", PermissionUnsafeStdlib); !ok {
		t.Error("revoking one permission took the others with it")
	}

	// A revocation is not a refusal: nothing may be recorded in its place, or
	// the plugin is dead with no way to revive it.
	reloaded := LoadPermissionStore(path)
	if _, ok := reloaded.Decision("notes", PermissionFFI); ok {
		t.Error("the revocation did not reach the file")
	}
	if _, ok := reloaded.Decision("notes", PermissionUnsafeStdlib); !ok {
		t.Error("the surviving grant did not reach the file")
	}
}

func TestRevokingWhatWasNeverGrantedIsHarmless(t *testing.T) {
	store := LoadPermissionStore(filepath.Join(t.TempDir(), "perms.json"))
	if err := store.Revoke("never-seen", PermissionFFI); err != nil {
		t.Fatalf("revoking what was never granted: %v", err)
	}
	if len(store.Grants()) != 0 {
		t.Error("revoking an unknown grant invented one")
	}
}

func TestPermissionGrantLineReadsAsAnAnswer(t *testing.T) {
	line := PermissionGrantLine(PermissionGrant{
		Plugin: "notes", Permission: PermissionFFI, Decision: PermissionAllow,
	})
	for _, want := range []string{"notes", "allowed to", "native system libraries"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line does not mention %q: %s", want, line)
		}
	}
}

func TestPluginPermissionsDialogListsAndRevokes(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	store := LoadPermissionStore(filepath.Join(t.TempDir(), "perms.json"))
	if err := store.Remember("notes", PermissionFFI, PermissionAllow); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	actionPluginPermissions(store)

	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(vtui.Container)
	if !ok {
		t.Fatalf("the dialog is a %T", top)
	}
	vtui.AssertLayout(t, dlg)

	var list *vtui.ListBox
	var revoke *vtui.Button
	for _, item := range dlg.GetChildren() {
		switch widget := item.(type) {
		case *vtui.ListBox:
			list = widget
		case *vtui.Button:
			if strings.Contains(widget.GetText(), "Revoke") {
				revoke = widget
			}
		}
	}
	if list == nil || revoke == nil {
		t.Fatal("the dialog has no list or no Revoke button")
	}
	if len(list.Items) != 1 || !strings.Contains(list.Items[0], "notes") {
		t.Fatalf("the dialog lists %v", list.Items)
	}

	revoke.OnClick()
	if confirm, _ := vtui.FrameManager.GetTopFrame().(*vtui.Window); confirm != nil && confirm.OnResult != nil {
		confirm.OnResult(0)
	}

	if _, ok := store.Decision("notes", PermissionFFI); ok {
		t.Error("the grant survived being revoked from the dialog")
	}
	if len(list.Items) != 1 || !strings.Contains(list.Items[0], "Nothing") {
		t.Errorf("the emptied dialog shows %v", list.Items)
	}
}

// TestPluginPermissionsDialogWithNothingGranted is the case a first time user
// meets: the dialog has to say something, and Revoke has to do nothing rather
// than reach past the end of an empty list.
func TestPluginPermissionsDialogWithNothingGranted(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	store := LoadPermissionStore(filepath.Join(t.TempDir(), "perms.json"))
	actionPluginPermissions(store)

	dlg, ok := vtui.FrameManager.GetTopFrame().(vtui.Container)
	if !ok {
		t.Fatal("the dialog did not open")
	}
	vtui.AssertLayout(t, dlg)

	for _, item := range dlg.GetChildren() {
		if button, ok := item.(*vtui.Button); ok && strings.Contains(button.GetText(), "Revoke") {
			button.OnClick()
		}
	}
	if len(store.Grants()) != 0 {
		t.Error("revoking with nothing granted changed the store")
	}
}
