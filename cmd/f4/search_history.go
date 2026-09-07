package main

import "github.com/unxed/vtui"

// History bucket names follow Far's SavedDialogHistory naming. Sharing the
// buckets is the point: a string typed into the editor's search dialog shows
// up in the viewer and in the file search dialog too, exactly like in Far.
const (
	searchTextHistoryID  = "SearchText"
	replaceTextHistoryID = "ReplaceText"
	fileMasksHistoryID   = "Masks"

	copyDestHistoryID       = "Copy"
	newFolderHistoryID      = "NewFolder"
	newEditHistoryID        = "NewEdit"
	externalEditorHistoryID = "ExternalEditor"
)

// attachHistory turns a plain dialog input into a history backed one, the
// equivalent of Far's DIF_HISTORY. The stored entries are loaded eagerly
// because Edit.HistoryUp and Edit.HistoryDown (Ctrl+E / Ctrl+X) walk the
// local slice; only the Ctrl+Down drop-down re-reads the provider itself.
func attachHistory(edit *vtui.Edit, historyID string) *vtui.Edit {
	if edit == nil || historyID == "" {
		return edit
	}
	edit.HistoryID = historyID
	edit.ShowHistoryButton = true
	edit.DeduplicateHistory = true
	if vtui.GlobalHistoryProvider != nil {
		edit.History = vtui.GlobalHistoryProvider.LoadHistory(historyID)
	}
	return edit
}

// attachHistoryUseLast is attachHistory plus Far's DIF_USELASTHISTORY: when
// the field would otherwise open empty it starts out holding the most recent
// entry. The text is selected, so the first keystroke replaces it instead of
// appending to it.
func attachHistoryUseLast(edit *vtui.Edit, historyID string) *vtui.Edit {
	attachHistory(edit, historyID)
	if edit == nil || edit.GetText() != "" || len(edit.History) == 0 {
		return edit
	}
	edit.SetText(edit.History[0])
	edit.SelectAll()
	return edit
}

// commitHistory records an accepted value. Deduplication, the length limit
// and persistence all live in Edit.AddHistory; this only keeps the call sites
// from having to re-check that the field exists and is history backed.
func commitHistory(edit *vtui.Edit, value string) {
	if edit == nil || edit.HistoryID == "" || value == "" {
		return
	}
	edit.AddHistory(value)
}

// inputBoxEdit returns the single Edit laid out by vtui.InputBox, which has no
// history support of its own. Locating the field here beats forking InputBox
// in vtui just to thread a history name through it.
func inputBoxEdit(dlg *vtui.Window) *vtui.Edit {
	if dlg == nil {
		return nil
	}
	for _, child := range dlg.GetChildren() {
		if edit, ok := child.(*vtui.Edit); ok {
			return edit
		}
	}
	return nil
}
