package app

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// withTempAssociations redirects panel.AssociationsFilePath to a fresh temp
// file for the duration of the test. Restores the default resolver on
// cleanup so subsequent tests aren't affected.
func withTempAssociations(t *testing.T, list []panel.FileAssoc) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "associations.ini")
	prev := panel.AssociationsFilePathFn
	panel.AssociationsFilePathFn = func() string { return path }
	t.Cleanup(func() { panel.AssociationsFilePathFn = prev })
	if list != nil {
		if err := panel.SaveAssociations(path, list); err != nil {
			t.Fatalf("prepare associations file: %v", err)
		}
	}
	return path
}

// setupPanelWithFile stages a panel.PanelsFrame whose active panel has the
// cursor on a single file entry named `name` inside a real temp dir.
// Returns the frame + the mock term.PTY so tests can observe writes.
func setupPanelWithFile(t *testing.T, name string) (*panel.PanelsFrame, *paneltest.MockPty) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	tmpDir := t.TempDir()
	cwd, err := os.Getwd()
	if err == nil {
		_ = os.Chdir(tmpDir)
		t.Cleanup(func() {
			_ = os.Chdir(cwd)
		})
	}

	if name != ".." && name != "" {
		fullPath := filepath.Join(tmpDir, name)
		if name == "some_subdir" {
			_ = os.MkdirAll(fullPath, 0700)
		} else {
			_ = os.WriteFile(fullPath, []byte("mock"), 0600)
		}
	}

	pf := paneltest.SetupMockPanelsFrame(t)
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)

	fsp := pf.Panels[pf.ActiveIdx].(*panel.FileSystemPanel)
	if err := fsp.Vfs.SetPath(tmpDir); err != nil {
		t.Fatal(err)
	}
	fsp.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: name}},
	}
	fsp.Refresh()
	fsp.SetCursorIndex(1)
	return pf, pf.Pty.(*paneltest.MockPty)
}

// TestFileAssociation_SingleMatch_RunsDirectly is the happy path:
// exactly one association fires for the file → its command reaches
// the term.PTY without any picker in between.
func TestFileAssociation_SingleMatch_RunsDirectly(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:        "*.png",
			Description: "Image viewer",
			Commands:    [panel.AssocKindCount]string{panel.AssocExecute: "eog !.!"},
			Enabled:     [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "pic.png")
	if !panel.TryFileAssociation(pf, panel.AssocExecute) {
		t.Fatal("expected tryFileAssociation to intercept when a match exists")
	}
	written := string(pty.Written)
	if !strings.Contains(written, "eog pic.png") {
		t.Errorf("PTY did not receive substituted command; got %q", written)
	}
}

// TestFileAssociation_NoMatch_FallsThrough verifies the intercept
// declines when nothing matches — the caller then runs its default
// behaviour (spawn / xdg-open / built-in viewer).
func TestFileAssociation_NoMatch_FallsThrough(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocExecute: "eog !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
	})

	pf, _ := setupPanelWithFile(t, "readme.txt")
	if panel.TryFileAssociation(pf, panel.AssocExecute) {
		t.Error("no PNG matches for readme.txt — intercept should return false")
	}
}

// TestFileAssociation_DisabledSlot_NotRun ensures a slot with the
// checkbox unchecked doesn't fire even when the command is populated.
func TestFileAssociation_DisabledSlot_NotRun(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocExecute: "eog !.!"},
			// Enabled all false.
		},
	})

	pf, _ := setupPanelWithFile(t, "pic.png")
	if panel.TryFileAssociation(pf, panel.AssocExecute) {
		t.Error("disabled slot must not intercept")
	}
}

// TestFileAssociation_WrongKind_NotRun ensures a slot enabled for
// View doesn't fire for an Execute lookup, and vice versa.
func TestFileAssociation_WrongKind_NotRun(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocView: "feh !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocView: true},
		},
	})

	pf, _ := setupPanelWithFile(t, "pic.png")
	if panel.TryFileAssociation(pf, panel.AssocExecute) {
		t.Error("View-only association must not intercept Execute")
	}
	if !panel.TryFileAssociation(pf, panel.AssocView) {
		t.Error("View-only association should intercept View lookup")
	}
}

// TestFileAssociation_Directory_NotIntercepted keeps directories in
// the default dispatch (cd for Enter, size calc for F3, attributes
// for F4). Associations only care about files.
func TestFileAssociation_Directory_NotIntercepted(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*",
			Commands: [panel.AssocKindCount]string{panel.AssocExecute: "echo hit"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
	})

	pf, _ := setupPanelWithFile(t, "some_subdir")
	fsp := pf.Panels[pf.ActiveIdx].(*panel.FileSystemPanel)
	// Flip the cursor's entry to a directory.
	fsp.Entries[1].IsDir = true

	if panel.TryFileAssociation(pf, panel.AssocExecute) {
		t.Error("directories must never be intercepted by associations")
	}
}

// TestFileAssociation_MultipleMatches_ShowsPicker checks the multi-
// match UX: no direct execution, a VMenu appears on top of the frame
// stack listing the candidates.
func TestFileAssociation_MultipleMatches_ShowsPicker(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:        "*.png",
			Description: "First",
			Commands:    [panel.AssocKindCount]string{panel.AssocExecute: "eog !.!"},
			Enabled:     [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
		{
			Mask:        "*.png",
			Description: "Second",
			Commands:    [panel.AssocKindCount]string{panel.AssocExecute: "feh !.!"},
			Enabled:     [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "pic.png")

	if !panel.TryFileAssociation(pf, panel.AssocExecute) {
		t.Fatal("expected intercept with 2 matches")
	}

	// Two matches must show a picker, not run either command yet.
	if len(pty.Written) != 0 {
		t.Errorf("PTY must be untouched until the user picks; got %q", pty.Written)
	}
	top := vtui.FrameManager.GetTopFrame()
	if top == nil || top.GetType() != vtui.TypeMenu {
		t.Fatalf("expected VMenu (picker) on top, got %#v", top)
	}
	menu, ok := top.(*vtui.VMenu)
	if !ok {
		t.Fatalf("top frame not *VMenu: %T", top)
	}
	if len(menu.Items) != 2 {
		t.Errorf("picker should list both matches; got %d rows", len(menu.Items))
	}
}

// TestFileAssociation_PickerRunsChosenCommand walks a full user flow:
// picker appears, user hits Enter on row 1, term.PTY gets the second
// command (verifying UserData routing).
func TestFileAssociation_PickerRunsChosenCommand(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:        "*.png",
			Description: "First",
			Commands:    [panel.AssocKindCount]string{panel.AssocExecute: "eog !.!"},
			Enabled:     [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
		{
			Mask:        "*.png",
			Description: "Second",
			Commands:    [panel.AssocKindCount]string{panel.AssocExecute: "feh !.!"},
			Enabled:     [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "pic.png")
	panel.TryFileAssociation(pf, panel.AssocExecute)

	top := vtui.FrameManager.GetTopFrame()
	menu := top.(*vtui.VMenu)
	menu.SetSelectPos(1) // pick "Second"
	// Fire OnAction to simulate Enter (the callback is what routes the
	// pick to the run). Then drain the task queue: the OnAction posts a
	// task that runs the command, and panel.ExecuteMenuCommands may enqueue
	// further tasks on its way to the term.PTY.
	menu.OnAction(1)
	// Drain the queue with a short timeout: PostTask hands off to an
	// internal goroutine, so a plain non-blocking select races the
	// goroutine's cycle. The timeout wins once the queue is truly empty.
drain:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(50 * time.Millisecond):
			break drain
		}
	}

	written := string(pty.Written)
	if !strings.Contains(written, "feh pic.png") {
		t.Errorf("PTY did not receive the picked command; got %q", written)
	}
	if strings.Contains(written, "eog") {
		t.Errorf("PTY erroneously received the unpicked command; got %q", written)
	}
}

// TestFileAssociation_ExcludeMask exercises the far2l "|" exclude
// syntax end-to-end through the dispatcher — a file matching the
// include but also the exclude must NOT trigger the association.
func TestFileAssociation_ExcludeMask(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png|thumb_*",
			Commands: [panel.AssocKindCount]string{panel.AssocView: "feh !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocView: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "thumb_pic.png")
	if panel.TryFileAssociation(pf, panel.AssocView) {
		t.Error("thumb_* exclude should veto pic.png match")
	}
	if len(pty.Written) != 0 {
		t.Errorf("PTY should be untouched; got %q", pty.Written)
	}
}

// TestFileAssociation_EditorAddSavesAndReloads exercises the editor's
// "Ins → fill fields → Save" flow: after Save the file on disk holds
// exactly what the editor entered.
func TestFileAssociation_EditorAddSavesAndReloads(t *testing.T) {
	path := withTempAssociations(t, nil)

	pf, _ := setupPanelWithFile(t, "anything")
	s := &panel.AssocEditorState{Pf: pf, SourcePath: path}
	s.EditAt(0, true)

	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(*vtui.Window)
	if !ok {
		t.Fatalf("expected edit dialog (*Window) on top, got %T", top)
	}

	// Poke values into the dialog's edits/checkboxes by walking the
	// registered children in the order editAt added them:
	// [Edit Mask, Edit Description, then 6× (Checkbox, Edit), ok, cancel,
	//  then two label widgets which we ignore].
	edits := []*vtui.Edit{}
	checks := []*vtui.Checkbox{}
	for _, it := range dlg.GetChildren() {
		switch v := it.(type) {
		case *vtui.Edit:
			edits = append(edits, v)
		case *vtui.Checkbox:
			checks = append(checks, v)
		}
	}
	if len(edits) < 2+panel.AssocKindCount {
		t.Fatalf("edit dialog missing edits, got %d", len(edits))
	}
	if len(checks) != panel.AssocKindCount {
		t.Fatalf("edit dialog missing checkboxes, got %d", len(checks))
	}
	edits[0].SetText("*.md,*.markdown")
	edits[1].SetText("Markdown")
	// Enable panel.AssocExecute (idx 0) with a command.
	checks[panel.AssocExecute].State = 1
	edits[2+panel.AssocExecute].SetText("view-md !.!")

	// Find and click the Save button.
	var saved *vtui.Button
	for _, it := range dlg.GetChildren() {
		if b, ok := it.(*vtui.Button); ok && b.IsDefault {
			saved = b
			break
		}
	}
	if saved == nil {
		t.Fatal("could not locate Save button")
	}
	saved.OnClick()

	// Read the file back and assert.
	loaded, err := panel.LoadAssociations(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry after save, got %d", len(loaded))
	}
	if loaded[0].Mask != "*.md,*.markdown" {
		t.Errorf("mask = %q, want %q", loaded[0].Mask, "*.md,*.markdown")
	}
	if loaded[0].Description != "Markdown" {
		t.Errorf("description = %q", loaded[0].Description)
	}
	if !loaded[0].Enabled[panel.AssocExecute] {
		t.Error("AssocExecute should be enabled")
	}
	if got := loaded[0].Commands[panel.AssocExecute]; got != "view-md !.!" {
		t.Errorf("execute cmd = %q", got)
	}
}

// TestFileAssociation_LangKeysResolve is the paranoia guard: every
// menu string we added to en.lng must resolve (i18n.Msg returns something
// that is not the fallback "{Key}" marker). Catches typos in either
// the code or the lang file.
func TestFileAssociation_LangKeysResolve(t *testing.T) {
	keys := []string{
		"Action.Panel.FileAssociations",
		"Action.Panel.FileAssociations.Desc",
		"FileAssoc.EditorTitle",
		"FileAssoc.EditTitle",
		"FileAssoc.NewTitle",
		"FileAssoc.ErrorTitle",
		"FileAssoc.EmptyHint",
		"FileAssoc.EmptyMask",
		"FileAssoc.MaskLabel",
		"FileAssoc.DescLabel",
		"FileAssoc.PickTitle.Open",
		"FileAssoc.PickTitle.View",
		"FileAssoc.PickTitle.Edit",
		"FileAssoc.DeleteTitle",
		"FileAssoc.DeleteConfirm",
	}
	// i18n.Msg() returns "{key}" when the key is missing (see lang_packs.go).
	for _, k := range keys {
		got := i18n.Msg(k)
		if strings.HasPrefix(got, "{") && strings.HasSuffix(got, "}") {
			t.Errorf("lang key %q not defined: got %q", k, got)
		}
	}
	// Silence "runtime unused" if this is the only reference.
	_ = runtime.GOOS
}

// TestFileAssociation_DeleteConfirmIsWarning is the regression guard
// for the associations-editor half of #379: the delete-entry
// confirmation is destructive and must render on the red WarnDialog
// palette, matching the file-manager delete confirmation.
func TestFileAssociation_DeleteConfirmIsWarning(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:        "*.md",
			Description: "Markdown",
			Commands:    [panel.AssocKindCount]string{panel.AssocExecute: "cat !.!"},
			Enabled:     [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
	})
	pf, _ := setupPanelWithFile(t, "readme.md")
	panel.ShowFileAssociations(pf)

	top := vtui.FrameManager.GetTopFrame()
	umf, ok := top.(*panel.UserMenuFrame)
	if !ok {
		t.Fatalf("expected *userMenuFrame on top after ShowFileAssociations, got %T", top)
	}
	umf.SetSelectPos(0)

	// Simulate the Del keypress the editor listens for.
	handled := umf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_DELETE,
	})
	if !handled {
		t.Fatal("Del was not consumed by the associations editor")
	}

	top = vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(*vtui.Window)
	if !ok {
		t.Fatalf("expected confirmation dialog (*Window) on top, got %T", top)
	}
	if !dlg.IsWarning {
		t.Error("Delete-association confirmation must render on the WarnDialog palette (see #379)")
	}
}
