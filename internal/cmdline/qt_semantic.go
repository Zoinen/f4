package cmdline

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func (cl *CommandLine) SemanticModel(ctx *vtui.SemanticContext) *extui.CommandLineModel {
	text := cl.Edit.GetText()
	cursorPosition, selectionStart, selectionEnd := semantic.SemanticEditPositions(cl.Edit, text)
	model := &extui.CommandLineModel{
		ID:             vtui.SemanticID(cl),
		Visible:        cl.IsVisible(),
		Focused:        cl.IsFocused(),
		Prompt:         cl.Prompt,
		PromptRuns:     semantic.RunsFromCells(cl.RichPrompt),
		Text:           text,
		Empty:          cl.IsEmpty(),
		InputX:         cl.Edit.X1 - cl.X1,
		CursorPosition: cursorPosition,
		SelectionStart: selectionStart,
		SelectionEnd:   selectionEnd,
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
