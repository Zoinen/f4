package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// aiUndoTestStack swaps in an empty undo stack for one test.
func aiUndoTestStack(t *testing.T) {
	t.Helper()
	aiUndoMu.Lock()
	saved := aiUndoStack
	aiUndoStack = nil
	aiUndoMu.Unlock()
	t.Cleanup(func() {
		aiUndoMu.Lock()
		aiUndoStack = saved
		aiUndoMu.Unlock()
	})
}

// aiUndoTestApply applies a two-file patch for real in a fresh folder and
// returns the folder and the run's transaction.
func aiUndoTestApply(t *testing.T) (string, *ap.Undo) {
	t.Helper()
	root := t.TempDir()
	for name, content := range map[string]string{"a.txt": "line1\nline2\n", "b.txt": "keep\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	patchPath := filepath.Join(t.TempDir(), "p.ap")
	patch := "ee000001 AP 3.2\n\n" +
		"ee000001 FILE\na.txt\n\nee000001 REPLACE\nee000001 snippet\nline1\nee000001 content\nLINE1\n\n" +
		"ee000001 FILE\nb.txt\n\nee000001 DELETE\n\n" +
		"ee000001 FILE\nsrc/new.txt\n\nee000001 CREATE\nee000001 content\nnew\n"
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatal(err)
	}
	res := ap.Apply(patchPath, root, ap.Options{Silent: true})
	if res.Status != ap.StatusSuccess || res.Undo == nil {
		t.Fatalf("apply = %s %v, undo %v; want SUCCESS with an Undo", res.Status, res.Error, res.Undo)
	}
	return root, res.Undo
}

// aiUndoTestAssertRestored checks root is back to what aiUndoTestApply found.
func aiUndoTestAssertRestored(t *testing.T, root string) {
	t.Helper()
	if got := readFileString(t, filepath.Join(root, "a.txt")); got != "line1\nline2\n" {
		t.Errorf("a.txt = %q, want the original", got)
	}
	if got := readFileString(t, filepath.Join(root, "b.txt")); got != "keep\n" {
		t.Errorf("b.txt = %q, want it back", got)
	}
	if _, err := os.Stat(filepath.Join(root, "src")); !os.IsNotExist(err) {
		t.Errorf("src/ the patch created is still there (%v)", err)
	}
}

func readFileString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// aiTopWindow is the top frame as the message window it must be.
func aiTopWindow(t *testing.T) *vtui.Window {
	t.Helper()
	w, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want a message window", vtui.FrameManager.GetTopFrame())
	}
	return w
}

func aiCloseTop() {
	if top := vtui.FrameManager.GetTopFrame(); top != nil {
		top.Close()
		vtui.FrameManager.RemoveFrame(top)
	}
}

func TestAIUndoStackDepth(t *testing.T) {
	aiUndoTestStack(t)
	aiPushUndo(nil)
	if aiTopUndo() != nil {
		t.Fatal("pushing nil recorded a transaction")
	}
	var all []*ap.Undo
	for i := 0; i < aiUndoDepth+5; i++ {
		u := &ap.Undo{}
		all = append(all, u)
		aiPushUndo(u)
	}
	if n := len(aiUndoStack); n != aiUndoDepth {
		t.Fatalf("stack holds %d transactions, want %d", n, aiUndoDepth)
	}
	if aiUndoStack[0] != all[5] || aiTopUndo() != all[len(all)-1] {
		t.Fatal("the stack must keep the newest transactions, newest on top")
	}
	aiDropUndo(all[len(all)-1])
	if aiTopUndo() != all[len(all)-2] {
		t.Fatal("dropping the top did not expose the one below")
	}
	aiDropUndo(&ap.Undo{}) // not on the stack: no-op
	if n := len(aiUndoStack); n != aiUndoDepth-1 {
		t.Fatalf("dropping an unknown transaction changed the stack to %d", n)
	}
}

// TestAIShowPatchResultUndo: a real run's result message carries Undo,
// which reverts that run at once, and the stack forgets it.
func TestAIShowPatchResultUndo(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)
	aiUndoTestStack(t)

	root, undo := aiUndoTestApply(t)
	aiPushUndo(undo)
	aiShowPatchResult(nil, root, false, 0, "  + SUCCESS", undo)
	dlg := aiTopWindow(t)
	t.Logf("result screen dump:\n%s", aiScreenText(t, scr, dlg))
	btn := aiReviewButtons(dlg)[aiCaption("AI.BtnUndoPatch")]
	if btn == nil {
		t.Fatal("the result of a real run lacks the Undo button")
	}
	btn.OnClick()
	aiUndoTestAssertRestored(t, root)
	if aiTopUndo() != nil {
		t.Fatal("a reverted transaction stayed on the stack")
	}
	done := aiScreenText(t, scr, aiTopWindow(t))
	t.Logf("after Undo:\n%s", done)
	if !strings.Contains(done, strings.SplitN(i18n.Msg("AI.UndoDone"), ":", 2)[0]) {
		t.Errorf("no %q message after Undo", i18n.Msg("AI.UndoDone"))
	}
	aiCloseTop()

	// A dry run, a failed run or a run with nothing written has no Undo.
	aiShowPatchResult(nil, root, true, 0, "", nil)
	if aiReviewButtons(aiTopWindow(t))[aiCaption("AI.BtnUndoPatch")] != nil {
		t.Fatal("a result without a transaction offers Undo")
	}
	aiCloseTop()
}

// TestAIUndoPatchConfirmAndConflict walks Ctrl+Z's path: nothing to undo,
// a confirmation listing the paths, a refusal after a later edit (the
// transaction stays), and the undo once the edit is gone.
func TestAIUndoPatchConfirmAndConflict(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)
	aiUndoTestStack(t)

	aiUndoPatch(nil)
	if text := aiScreenText(t, scr, aiTopWindow(t)); !strings.Contains(text, i18n.Msg("AI.UndoNothing")) {
		t.Fatalf("empty stack: no %q message:\n%s", i18n.Msg("AI.UndoNothing"), text)
	}
	aiCloseTop()

	root, undo := aiUndoTestApply(t)
	aiPushUndo(undo)
	patched := readFileString(t, filepath.Join(root, "a.txt"))
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	aiUndoPatch(nil)
	confirm := aiTopWindow(t)
	text := aiScreenText(t, scr, confirm)
	t.Logf("confirmation:\n%s", text)
	for _, want := range []string{"a.txt", "b.txt", "src"} {
		if !strings.Contains(text, want) {
			t.Errorf("confirmation lacks %q", want)
		}
	}
	aiReviewButtons(confirm)[aiCaption("AI.BtnUndoPatch")].OnClick()
	refusal := aiScreenText(t, scr, aiTopWindow(t))
	t.Logf("refusal:\n%s", refusal)
	if head := strings.SplitN(fmt.Sprintf(i18n.Msg("AI.UndoConflict"), 1), ",", 2)[0]; !strings.Contains(refusal, head) {
		t.Errorf("no conflict message %q", head)
	}
	if got := readFileString(t, filepath.Join(root, "a.txt")); got != "edited by hand\n" {
		t.Fatalf("a refused undo touched a.txt: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "b.txt")); !os.IsNotExist(err) {
		t.Fatal("a refused undo restored b.txt")
	}
	if aiTopUndo() != undo {
		t.Fatal("a refused undo dropped the transaction")
	}
	aiCloseTop()

	// Cancel on the confirmation does nothing.
	aiUndoPatch(nil)
	aiReviewButtons(aiTopWindow(t))[aiCaption("vtui.Cancel")].OnClick()
	if aiTopUndo() != undo {
		t.Fatal("Cancel dropped the transaction")
	}
	aiCloseTop()

	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(patched), 0o600); err != nil {
		t.Fatal(err)
	}
	aiUndoPatch(nil)
	aiReviewButtons(aiTopWindow(t))[aiCaption("AI.BtnUndoPatch")].OnClick()
	aiUndoTestAssertRestored(t, root)
	if aiTopUndo() != nil {
		t.Fatal("the reverted transaction stayed on the stack")
	}
	aiCloseTop()
}

func TestAIChatPanelCtrlZUndoes(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	aiUndoTestStack(t)
	fp := panel.NewFileSystemPanel(0, 0, 80, 24, vfs.NewNullVFS(0))
	paneltest.WaitForLoad(t, fp)
	cp := NewAIChatPanel(fp)
	cp.SetFocus(true)

	root, undo := aiUndoTestApply(t)
	aiPushUndo(undo)
	for _, ctrl := range []vtinput.ControlKeyState{vtinput.LeftCtrlPressed, vtinput.RightCtrlPressed} {
		if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_Z, ControlKeyState: ctrl}) {
			t.Fatal("Ctrl+Z was not handled by the AI panel")
		}
		if aiReviewButtons(aiTopWindow(t))[aiCaption("AI.BtnUndoPatch")] == nil {
			t.Fatal("Ctrl+Z did not ask to undo the last patch")
		}
		aiCloseTop()
	}
	cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_Z, ControlKeyState: vtinput.LeftCtrlPressed})
	aiReviewButtons(aiTopWindow(t))[aiCaption("AI.BtnUndoPatch")].OnClick()
	aiUndoTestAssertRestored(t, root)
	aiCloseTop()
}

// The apply confirmation mentions git only for a project that is under it.
func TestAIInGitWorkTree(t *testing.T) {
	plain := t.TempDir()
	if aiInGitWorkTree(plain) {
		t.Fatal("a plain temp dir reported as under git")
	}
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if !aiInGitWorkTree(repo) || !aiInGitWorkTree(sub) {
		t.Fatal("a folder with .git, or below one, not reported as under git")
	}
}
