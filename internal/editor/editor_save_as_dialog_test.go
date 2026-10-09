package editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestSaveAsCodepages checks the pure list-building behind the Save As
// dialog's codepage combo box: every available codepage in order, its
// trimmed menu label, and the index of the requested codepage.
func TestSaveAsCodepages(t *testing.T) {
	if len(vfs.AvailableCodepages) == 0 {
		t.Skip("no codepages registered in this environment")
	}
	want := vfs.AvailableCodepages[len(vfs.AvailableCodepages)/2]

	cps, labels, idx := saveAsCodepages(want.ID)
	if len(cps) != len(vfs.AvailableCodepages) {
		t.Fatalf("got %d codepages, want %d", len(cps), len(vfs.AvailableCodepages))
	}
	if len(labels) != len(cps) {
		t.Fatalf("got %d labels for %d codepages", len(labels), len(cps))
	}
	if idx < 0 || idx >= len(cps) || vfs.NormalizeCodepageID(cps[idx].ID) != vfs.NormalizeCodepageID(want.ID) {
		t.Fatalf("index %d does not select codepage %d, got %+v", idx, want.ID, cps[idx])
	}
	for i, cp := range cps {
		if labels[i] != strings.TrimSpace(vfs.CodepageMenuLabel(cp)) {
			t.Errorf("label[%d] = %q, want trimmed menu label of %+v", i, labels[i], cp)
		}
	}

	// An id that matches nothing in the list still returns a usable index
	// (the first entry) instead of -1, so the combo box always has a
	// selection.
	if _, _, idx := saveAsCodepages(-999999); idx != 0 {
		t.Errorf("unknown codepage id: index = %d, want 0", idx)
	}
}

// TestResolveSaveAsPath_NonLocalVFSAndNoBasePath covers the two branches of
// resolveSaveAsPath that the round-trip save tests never reach: a relative
// name typed while the editor holds a virtual VFS (e.g. Terminal Log), and
// one typed with no file open yet (no directory to anchor it to).
func TestResolveSaveAsPath_NonLocalVFSAndNoBasePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openLocalEditor(t, dir, path)
	defer ev.Close()

	// A relative name on a virtual VFS means nothing to that VFS's own
	// Stat/Create, so it is resolved against the real OS current directory
	// instead.
	ev.Vfs = vfs.NewNullVFS(0)
	want, err := vfs.NewOSVFS("").Abs("rel.txt")
	if err != nil {
		t.Fatalf("reference Abs: %v", err)
	}
	if got := ev.resolveSaveAsPath("rel.txt"); got != want {
		t.Errorf("non-local VFS: resolveSaveAsPath(%q) = %q, want %q", "rel.txt", got, want)
	}

	// A local VFS but no file open yet: nothing to take a directory from,
	// so a relative name resolves against the VFS's own current path.
	ev.Vfs = vfs.NewOSVFS(dir)
	ev.FilePath = ""
	want2 := filepath.Join(dir, "rel.txt")
	if got := ev.resolveSaveAsPath("rel.txt"); got != want2 {
		t.Errorf("no base path: resolveSaveAsPath(%q) = %q, want %q", "rel.txt", got, want2)
	}
}

// saveAsDialogWidgets pulls out the interactive controls ShowSaveAsDialog
// builds, in the order it adds them: an Edit, a ComboBox, a Checkbox, a
// RadioGroup and two Buttons (Save, then Cancel).
func saveAsDialogWidgets(t *testing.T, w *vtui.Window) (editPath *vtui.Edit, comboCP *vtui.ComboBox, chkBOM *vtui.Checkbox, eolGroup *vtui.RadioGroup, btnSave, btnCancel *vtui.Button) {
	t.Helper()
	var buttons []*vtui.Button
	for _, c := range w.GetChildren() {
		switch v := c.(type) {
		case *vtui.Edit:
			editPath = v
		case *vtui.ComboBox:
			comboCP = v
		case *vtui.Checkbox:
			chkBOM = v
		case *vtui.RadioGroup:
			eolGroup = v
		case *vtui.Button:
			buttons = append(buttons, v)
		}
	}
	if editPath == nil || comboCP == nil || chkBOM == nil || eolGroup == nil || len(buttons) != 2 {
		t.Fatalf("Save As dialog is missing a widget: edit=%v combo=%v checkbox=%v radio=%v buttons=%d",
			editPath, comboCP, chkBOM, eolGroup, len(buttons))
	}
	return editPath, comboCP, chkBOM, eolGroup, buttons[0], buttons[1]
}

// TestShowSaveAsDialog_BOMFollowsCodepageChoice covers syncBOM: switching
// the codepage combo enables/disables and (un)checks the BOM checkbox
// according to whether the newly chosen codepage is Unicode, and, for UTF-8
// specifically, according to whether the file already carries a BOM.
func TestShowSaveAsDialog_BOMFollowsCodepageChoice(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openLocalEditor(t, dir, path)
	defer ev.Close()
	ev.Utf8BOM = false

	ev.ShowSaveAsDialog()
	top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || strings.TrimSpace(top.GetTitle()) != strings.TrimSpace(i18n.Msg("SaveAs.Title")) {
		t.Fatalf("Save As dialog did not open, top frame = %#v", vtui.FrameManager.GetTopFrame())
	}
	_, comboCP, chkBOM, _, _, btnCancel := saveAsDialogWidgets(t, top)

	_, _, idxAnsi := saveAsCodepages(1251)
	comboCP.Menu.SetSelectPos(idxAnsi)
	comboCP.Menu.OnAction(idxAnsi)
	if !chkBOM.IsDisabled() || chkBOM.State != 0 {
		t.Errorf("windows-1251 selected: BOM checkbox disabled=%v state=%d, want disabled/unchecked",
			chkBOM.IsDisabled(), chkBOM.State)
	}

	_, _, idxUtf16 := saveAsCodepages(1200)
	comboCP.Menu.SetSelectPos(idxUtf16)
	comboCP.Menu.OnAction(idxUtf16)
	if chkBOM.IsDisabled() || chkBOM.State != 1 {
		t.Errorf("UTF-16LE selected: BOM checkbox disabled=%v state=%d, want enabled/checked",
			chkBOM.IsDisabled(), chkBOM.State)
	}

	_, _, idxUtf8 := saveAsCodepages(65001)
	comboCP.Menu.SetSelectPos(idxUtf8)
	comboCP.Menu.OnAction(idxUtf8)
	if chkBOM.IsDisabled() || chkBOM.State != 0 {
		t.Errorf("UTF-8 selected on a file without a BOM: disabled=%v state=%d, want enabled/unchecked",
			chkBOM.IsDisabled(), chkBOM.State)
	}

	btnCancel.OnClick()
}

// TestShowSaveAsDialog_SaveAppliesDialogChoices drives the Shift+F2 dialog
// end to end through its own widgets (rather than calling ev.saveAs
// directly, as the round-trip tests do): pick a codepage, tick the BOM
// checkbox, pick line breaks, type a path, and click Save.
func TestShowSaveAsDialog_SaveAppliesDialogChoices(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(path, []byte("hi\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openLocalEditor(t, dir, path)
	defer ev.Close()

	ev.ShowSaveAsDialog()
	top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("Save As dialog did not open, top frame = %#v", vtui.FrameManager.GetTopFrame())
	}
	editPath, comboCP, chkBOM, eolGroup, btnSave, _ := saveAsDialogWidgets(t, top)

	_, _, idxUtf16 := saveAsCodepages(1200)
	comboCP.Menu.SetSelectPos(idxUtf16)
	comboCP.Menu.OnAction(idxUtf16)
	if chkBOM.IsDisabled() {
		t.Fatal("UTF-16LE must offer a BOM checkbox")
	}
	chkBOM.State = 1
	eolGroup.Selected = int(saveAsEOLUnix)
	target := filepath.Join(dir, "copy.txt")
	editPath.SetText(target)

	btnSave.OnClick()
	pumpEditorUntil(t, func() bool { return ev.FilePath == target && !ev.Saving && !ev.Modified })
	testutil.DrainPendingTasks()

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	want := []byte{0xFF, 0xFE, 'h', 0, 'i', 0, '\n', 0}
	if string(got) != string(want) {
		t.Errorf("target = %x, want %x", got, want)
	}
	if ev.Codepage != 1200 {
		t.Errorf("editor codepage = %d, want 1200", ev.Codepage)
	}
}

// TestShowSaveAsDialog_EmptyPathIsRejected covers the Save button's guard
// clause: an empty (or whitespace-only) path shows an error and leaves the
// dialog open instead of closing it and calling saveAs with nothing to
// write to.
func TestShowSaveAsDialog_EmptyPathIsRejected(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(path, []byte("hi\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openLocalEditor(t, dir, path)
	defer ev.Close()

	ev.ShowSaveAsDialog()
	top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("Save As dialog did not open, top frame = %#v", vtui.FrameManager.GetTopFrame())
	}
	editPath, _, _, _, btnSave, _ := saveAsDialogWidgets(t, top)
	editPath.SetText("   ")

	btnSave.OnClick()

	errBox, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || errBox == top {
		t.Fatalf("empty path did not raise the error dialog, top frame = %#v", vtui.FrameManager.GetTopFrame())
	}
	if strings.TrimSpace(errBox.GetTitle()) != strings.TrimSpace(i18n.Msg("SaveAs.Title")) {
		t.Errorf("error dialog title = %q, want %q", errBox.GetTitle(), i18n.Msg("SaveAs.Title"))
	}
	if ev.FilePath != path || ev.Saving {
		t.Errorf("empty path must not start a save: FilePath=%q Saving=%v", ev.FilePath, ev.Saving)
	}
	errBox.Close()
}
