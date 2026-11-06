package vtui

import "github.com/unxed/vtinput"

const (
	editUndoLimit = 128
	editUndoBytes = 8 << 20
)

// Snapshots own their text: editing the rune slice must not alter old states.
type editUndoState struct {
	text                                  string
	cursor, left, start, end, anchor, top int
	clear, block                          bool
	blockAnchorRow, blockAnchorColumn     int
	blockFocusRow, blockFocusColumn, wrap int
}

type editUndoChange struct {
	before, after editUndoState
	kind          string
}

func (e *Edit) undoState() editUndoState {
	return editUndoState{
		text: e.GetText(), cursor: e.curPos, left: e.leftPos,
		start: e.selStart, end: e.selEnd, anchor: e.selAnchor,
		top: e.multilineTop, clear: e.clearFlag, block: e.blockSelection,
		blockAnchorRow: e.blockAnchorRow, blockAnchorColumn: e.blockAnchorColumn,
		blockFocusRow: e.blockFocusRow, blockFocusColumn: e.blockFocusColumn, wrap: e.blockWrapWidth,
	}
}

// CanUndo and CanRedo expose capabilities without exposing the stored text.
func (e *Edit) CanUndo() bool {
	return !e.IsDisabled() && !e.PasswordMode && !e.pasting && len(e.undoChanges) > 0
}

func (e *Edit) CanRedo() bool {
	return !e.IsDisabled() && !e.PasswordMode && !e.pasting && len(e.redoChanges) > 0
}

func (e *Edit) resetUndo() {
	e.undoChanges = nil
	e.redoChanges = nil
	e.undoMerge = false
}

func (e *Edit) beginUndo(kind string) func() {
	if e.undoDepth > 0 {
		return func() {}
	}
	if e.PasswordMode {
		e.resetUndo()
		return func() {}
	}
	before := e.undoState()
	e.undoDepth++
	return func() {
		e.undoDepth--
		after := e.undoState()
		if before.text == after.text {
			return
		}
		merge := e.undoMerge && kind != "" && len(e.undoChanges) > 0
		if merge {
			last := &e.undoChanges[len(e.undoChanges)-1]
			merge = last.kind == kind && last.after.text == before.text && last.after.cursor == before.cursor
			merge = merge && before.start < 0 && !before.block && !before.clear
			if merge {
				last.after = after
			}
		}
		if !merge {
			e.undoChanges = append(e.undoChanges, editUndoChange{before: before, after: after, kind: kind})
		}
		e.redoChanges = nil
		e.undoMerge = kind != ""
		e.trimUndo()
		e.ScreenObject.NotifyChange()
	}
}

func (e *Edit) trimUndo() {
	bytes := 0
	keep := len(e.undoChanges)
	for i := len(e.undoChanges) - 1; i >= 0; i-- {
		change := e.undoChanges[i]
		bytes += len(change.before.text) + len(change.after.text)
		if bytes > editUndoBytes || keep-i > editUndoLimit {
			e.undoChanges = append([]editUndoChange(nil), e.undoChanges[i+1:]...)
			return
		}
	}
}

// Undo restores text, cursor and linear or rectangular selection together.
func (e *Edit) Undo() bool {
	if !e.CanUndo() {
		return false
	}
	i := len(e.undoChanges) - 1
	change := e.undoChanges[i]
	e.undoChanges[i] = editUndoChange{}
	e.undoChanges = e.undoChanges[:i]
	e.redoChanges = append(e.redoChanges, change)
	e.restoreUndo(change.before)
	DebugLog("[FIX:edit-undo] undo=%d redo=%d", len(e.undoChanges), len(e.redoChanges))
	return true
}

func (e *Edit) Redo() bool {
	if !e.CanRedo() {
		return false
	}
	i := len(e.redoChanges) - 1
	change := e.redoChanges[i]
	e.redoChanges[i] = editUndoChange{}
	e.redoChanges = e.redoChanges[:i]
	e.undoChanges = append(e.undoChanges, change)
	e.restoreUndo(change.after)
	DebugLog("[FIX:edit-undo] undo=%d redo=%d", len(e.undoChanges), len(e.redoChanges))
	return true
}

func (e *Edit) restoreUndo(state editUndoState) {
	e.text = []rune(state.text)
	e.curPos, e.leftPos = state.cursor, state.left
	e.selStart, e.selEnd, e.selAnchor = state.start, state.end, state.anchor
	e.clearFlag, e.blockSelection = state.clear, state.block
	e.blockAnchorRow, e.blockAnchorColumn = state.blockAnchorRow, state.blockAnchorColumn
	e.blockFocusRow, e.blockFocusColumn, e.blockWrapWidth = state.blockFocusRow, state.blockFocusColumn, state.wrap
	e.multilineTop = state.top
	e.multilineMoveActive = false
	e.multilineCacheText = ""
	e.multilineCache = nil
	e.multilineCacheWidth = 0
	e.undoMerge = false
	if e.OnTextChange != nil {
		e.OnTextChange(e.GetText())
	}
	e.NotifyChange()
}

// ReplaceText is a user-initiated replacement; SetText establishes a new baseline.
func (e *Edit) ReplaceText(text string) {
	defer e.beginUndo("")()
	e.SetText(text)
	e.ClearSelection()
}

// Completion previews are neither committed edits nor new document baselines.
func (e *Edit) setPreviewText(text string) {
	e.undoDepth++
	defer func() { e.undoDepth-- }()
	e.SetText(text)
}

func (e *Edit) undoKeyKind(event *vtinput.InputEvent) (string, bool) {
	if event.Type == vtinput.PasteEventType {
		return "", !event.PasteStart
	}
	if event.Type != vtinput.KeyEventType || !event.KeyDown || e.pasting || e.IsDisabled() {
		return "", false
	}
	ctrl := event.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := event.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := event.ControlKeyState&vtinput.ShiftPressed != 0
	if !alt && !ctrl && event.Char >= ' ' {
		return "type", true
	}
	if !alt {
		switch event.VirtualKeyCode {
		case vtinput.VK_BACK:
			return "backspace", true
		case vtinput.VK_DELETE:
			return "delete", true
		case vtinput.VK_X, vtinput.VK_V:
			return "", ctrl && !shift
		case vtinput.VK_INSERT:
			return "", shift && !ctrl
		}
	}
	return "", false
}
