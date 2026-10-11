package app

import (
	"errors"
	"fmt"
	"sync"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/vtui"
)

// Undoing an applied ap patch (docs/VTVIBE.md §7.4, f4#1606). Every real
// "Apply" that wrote something hands back its transaction (ap.Result.Undo);
// f4 keeps the last aiUndoDepth of them, newest on top, for this f4 process
// only. Ctrl+Z in the AI panel, "ai:undo" and Commands > AI > Undo AP patch
// revert the newest after a confirmation; the Undo button on the result
// message reverts the run it reports right away. The engine refuses when a
// file the patch touched has changed since - nothing is restored then, and
// the transaction stays on the stack, so putting the file back as the patch
// left it makes the undo possible again.

// aiUndoDepth is §7.4's default depth (undo_depth in vtvibe.ini, §15).
const aiUndoDepth = 20

var (
	aiUndoMu    sync.Mutex
	aiUndoStack []*ap.Undo
)

// aiPushUndo records a finished real run's transaction; nil is ignored.
func aiPushUndo(u *ap.Undo) {
	if u == nil {
		return
	}
	aiUndoMu.Lock()
	defer aiUndoMu.Unlock()
	aiUndoStack = append(aiUndoStack, u)
	if len(aiUndoStack) > aiUndoDepth {
		aiUndoStack = append([]*ap.Undo(nil), aiUndoStack[len(aiUndoStack)-aiUndoDepth:]...)
	}
}

// aiTopUndo is the newest transaction, nil when there is none.
func aiTopUndo() *ap.Undo {
	aiUndoMu.Lock()
	defer aiUndoMu.Unlock()
	if len(aiUndoStack) == 0 {
		return nil
	}
	return aiUndoStack[len(aiUndoStack)-1]
}

// aiDropUndo removes u from the stack once it is spent.
func aiDropUndo(u *ap.Undo) {
	aiUndoMu.Lock()
	defer aiUndoMu.Unlock()
	for i := len(aiUndoStack) - 1; i >= 0; i-- {
		if aiUndoStack[i] == u {
			aiUndoStack = append(aiUndoStack[:i:i], aiUndoStack[i+1:]...)
			return
		}
	}
}

// aiUndoPatch asks before reverting the newest applied patch: which folder
// and which paths it puts back.
func aiUndoPatch(pf *panel.PanelsFrame) {
	u := aiTopUndo()
	if u == nil {
		u = aiLoadSavedUndos(pf)
	}
	if u == nil {
		vtui.ShowMessage(i18n.Msg("AI.PatchTitle"), i18n.Msg("AI.UndoNothing"), []string{i18n.Msg("vtui.Ok")})
		return
	}
	paths := u.Paths()
	body := fmt.Sprintf(i18n.Msg("AI.UndoConfirm"), u.ProjectDir(), len(paths)) + aiPathList(paths)
	dlg := vtui.ShowMessage(i18n.Msg("AI.PatchTitle"), body,
		[]string{i18n.Msg("AI.BtnUndoPatch"), i18n.Msg("vtui.Cancel")})
	dlg.OnResult = func(code int) {
		if code == 0 {
			aiRevertPatch(pf, u)
		}
	}
}

// aiRevertPatch reverts u and says how it went.
func aiRevertPatch(pf *panel.PanelsFrame, u *ap.Undo) {
	err := u.Revert()
	var conflict *ap.UndoConflictError
	switch {
	case err == nil:
		aiDropUndo(u)
		pf.RefreshAll()
		vtui.ShowMessage(i18n.Msg("AI.PatchTitle"), i18n.Msg("AI.UndoDone"), []string{i18n.Msg("vtui.Ok")})
	case errors.As(err, &conflict):
		vtui.ShowMessage(i18n.Msg("AI.ErrorTitle"),
			fmt.Sprintf(i18n.Msg("AI.UndoConflict"), len(conflict.Paths))+aiPathList(conflict.Paths),
			[]string{i18n.Msg("vtui.Ok")})
	case u.Reverted():
		// Restoring started and hit an I/O error: some paths are back,
		// some are not, and the transaction cannot be tried again.
		aiDropUndo(u)
		pf.RefreshAll()
		vtui.ShowMessage(i18n.Msg("AI.ErrorTitle"), fmt.Sprintf(i18n.Msg("AI.UndoFailed"), err.Error()),
			[]string{i18n.Msg("vtui.Ok")})
	default:
		// A touched path could not even be read: nothing was restored.
		aiShowError(err)
	}
}

// aiPathList is the "\n  path" list under a confirmation, capped like the
// Apply confirmation's list of files.
func aiPathList(paths []string) string {
	shown := paths
	if len(shown) > 12 {
		shown = shown[:12]
	}
	var s string
	for _, p := range shown {
		s += "\n  " + p
	}
	if len(paths) > len(shown) {
		s += "\n  " + fmt.Sprintf(i18n.Msg("AI.PatchMoreFiles"), len(paths)-len(shown))
	}
	return s
}

// aiLoadSavedUndos brings back the transactions an earlier f4 left in the
// project's .vtvibe/undo/ when nothing is on the stack (a restart, or a
// project this run has not patched), and returns the newest. The project is
// the folder the next patch would be applied to.
func aiLoadSavedUndos(pf *panel.PanelsFrame) *ap.Undo {
	root, ok := aiPatchTargetDir(pf)
	if !ok {
		return nil
	}
	saved := ap.LoadSavedUndos(root)
	if len(saved) > aiUndoDepth {
		saved = saved[len(saved)-aiUndoDepth:]
	}
	for _, u := range saved {
		aiPushUndo(u)
	}
	return aiTopUndo()
}
