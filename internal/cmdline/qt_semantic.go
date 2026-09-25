package cmdline

import (
	"unicode/utf16"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// SetNativeCursor accepts Qt's UTF-16 offset; the shared editor owns rune and
// grapheme boundaries and subsequent keyboard editing in both frontends.
func (cl *CommandLine) SetNativeCursor(position int) {
	cl.SetNativeSelection(position, position)
}

// SetNativeSelection maps presentation UTF-16 offsets into the shared editor.
func (cl *CommandLine) SetNativeSelection(anchor, cursor int) {
	cl.Edit.HandleSemanticAction(map[string]any{
		"action": "control.select", "anchor": cl.nativeRuneOffset(anchor),
		"cursor": cl.nativeRuneOffset(cursor),
	})
	vtui.DebugLog("[FIX:command-line-selection] native anchor=%d cursor=%d", anchor, cursor)
}

// SetNativeBlockSelection maps Qt's UTF-16 endpoints into the shared edit
// buffer and keeps the selection rectangular in its visual multiline layout.
func (cl *CommandLine) SetNativeBlockSelection(anchor, cursor int) {
	cl.Edit.SetMultilineBlockSelection(cl.nativeRuneOffset(anchor), cl.nativeRuneOffset(cursor))
	vtui.DebugLog("[FIX:command-line-selection] native block anchor=%d cursor=%d", anchor, cursor)
}

// SetNativeBlockSelectionAt applies a rectangular selection using Qt's rendered
// row and cell coordinates instead of estimating them from the Go viewport.
func (cl *CommandLine) SetNativeBlockSelectionAt(anchor, cursor int, geometry vtui.MultilineBlockSelectionGeometry) {
	cl.Edit.SetMultilineBlockSelectionAt(
		cl.nativeRuneOffset(anchor), cl.nativeRuneOffset(cursor), geometry)
	vtui.DebugLog("[FIX:command-line-selection] native block rows=%d:%d columns=%d:%d width=%d",
		geometry.AnchorRow, geometry.FocusRow, geometry.AnchorColumn,
		geometry.FocusColumn, geometry.WrapWidth)
}

func (cl *CommandLine) nativeRuneOffset(position int) int {
	index, units := 0, 0
	for _, r := range cl.Edit.GetText() {
		units += utf16.RuneLen(r)
		if units > position {
			break
		}
		index++
	}
	return index
}

func (cl *CommandLine) SemanticModel(ctx *vtui.SemanticContext) *extui.CommandLineModel {
	cl.SyncInputOptions()
	text := cl.Edit.GetText()
	cursorPosition, selectionStart, selectionEnd := semantic.SemanticEditPositions(cl.Edit, text)
	block, blockAnchorRow, blockAnchorColumn, blockFocusRow, blockFocusColumn := cl.Edit.MultilineBlockSelection()
	if block {
		selectionStart, selectionEnd = -1, -1
	}
	model := &extui.CommandLineModel{
		ID:                vtui.SemanticID(cl),
		Visible:           cl.IsVisible(),
		Focused:           cl.IsFocused(),
		Multiline:         cl.Edit.Multiline,
		WordWrap:          cl.Edit.WordWrap,
		Prompt:            cl.Prompt,
		PromptRuns:        semantic.RunsFromCells(cl.RichPrompt),
		Text:              text,
		Empty:             cl.IsEmpty(),
		InputX:            cl.Edit.X1 - cl.X1,
		CursorPosition:    cursorPosition,
		SelectionStart:    selectionStart,
		SelectionEnd:      selectionEnd,
		BlockSelection:    block,
		BlockAnchorRow:    blockAnchorRow,
		BlockAnchorColumn: blockAnchorColumn,
		BlockFocusRow:     blockFocusRow,
		BlockFocusColumn:  blockFocusColumn,
	}
	rendered := semantic.SemanticRenderSurface(cl.X1, cl.Y1, cl.X2, cl.Y2, cl.DisplayObject)
	if len(rendered.Rows) > 0 {
		model.Runs = rendered.Rows[0]
	}
	model.CursorX = rendered.CursorX
	model.CursorPrefixRuns = rendered.CursorPrefixRuns
	model.CursorVisible = rendered.CursorVisible
	model.CursorShape = rendered.CursorShape
	return model
}
