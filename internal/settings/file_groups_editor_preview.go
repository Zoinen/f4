package settings

import (
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

var fileGroupColourFields = []string{
	"group.NormalColor", "group.SelectedColor", "group.CursorColor", "group.SelectedCursorColor",
}

func fileGroupColourState(id string) int {
	for i, field := range fileGroupColourFields {
		if fileGroupFieldKey(id) == fileGroupFieldKey(field) {
			return i
		}
	}
	return -1
}

func fileGroupStateLabels() []string {
	return []string{i18n.Msg("FileGroups.Normal"), i18n.Msg("FileGroups.Marked"),
		i18n.Msg("FileGroups.Cursor"), i18n.Msg("FileGroups.MarkedCursor")}
}

// Refreshed on valid draft edits, never while painting. The console preview
// stays outside the scrolling page and links to its existing color fields.
type fileGroupEditorSample struct {
	*vtui.Group
	preview fileGroupPreview
	catalog string
	problem string
	state   int
	edit    func(int)
}

func newFileGroupEditorSample() *fileGroupEditorSample {
	s := &fileGroupEditorSample{Group: vtui.NewGroup(0, 0, 1, 1)}
	s.SetId("filegroups:editor-preview")
	return s
}

func (s *fileGroupEditorSample) CanFocus() bool { return s.edit != nil && !s.IsDisabled() }

func (s *fileGroupEditorSample) ProcessKey(e *vtinput.InputEvent) bool {
	if !s.CanFocus() || !e.KeyDown || e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		s.state = max(0, s.state-1)
	case vtinput.VK_RIGHT:
		s.state = min(len(fileGroupColourFields)-1, s.state+1)
	case vtinput.VK_RETURN, vtinput.VK_SPACE, vtinput.VK_F4:
		s.edit(s.state)
	default:
		return false
	}
	return true
}

func (s *fileGroupEditorSample) ProcessMouse(e *vtinput.InputEvent) bool {
	if !s.CanFocus() || !s.HitTest(int(e.MouseX), int(e.MouseY)) || int(e.MouseY) > s.Y1+1 ||
		e.ButtonState != vtinput.FromLeft1stButtonPressed || !vtui.IsMousePress(e) {
		return false
	}
	width := s.X2 - s.X1 + 1
	s.state = ((int(e.MouseX)-s.X1+1)*len(fileGroupColourFields) - 1) / width
	s.edit(s.state)
	return true
}

func (s *fileGroupEditorSample) Show(scr *vtui.ScreenBuf) {
	width := s.X2 - s.X1 + 1
	count := len(fileGroupColourFields)
	for i, label := range fileGroupStateLabels() {
		x := s.X1 + i*width/count
		columns := max(0, s.X1+(i+1)*width/count-x-1)
		attr := vtui.Palette[vtui.ColDialogText]
		if s.IsFocused() && s.state == i {
			attr = vtui.Palette[vtui.ColDialogSelectedButton]
		}
		scr.Write(x, s.Y1, vtui.StringToCharInfo(vtui.TruncateString(label, columns, "…"), attr))
		name := s.preview.style.Marker + s.preview.item.Name
		text := vtui.TruncateString(name, columns, "…")
		text += strings.Repeat(" ", max(0, columns-vtui.StringWidth(text)))
		scr.Write(x, s.Y1+1, vtui.StringToCharInfo(text, s.preview.attrs[i]))
	}
	if s.problem != "" {
		scr.Write(s.X1, s.Y1+2, vtui.StringToCharInfo(
			vtui.TruncateString(s.problem, s.X2-s.X1+1, "…"), vtui.Palette[vtui.ColDialogHighlightText]))
	}
}

func (s *fileGroupEditorSample) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	n := s.Group.SemanticNode(ctx)
	n["fileGroupSample"] = true
	n["previewStyle"] = s.preview.style.ToMap()
	n["previewName"] = s.preview.item.Name
	n["previewCatalog"] = s.catalog
	n["previewDirectory"] = s.preview.item.IsDir
	n["previewHidden"] = s.preview.item.IsHidden
	n["items"] = fileGroupStateLabels()
	n["text"] = s.problem
	return n
}

func fileGroupColourPatches(style extui.HighlightStyleModel) []extui.HighlightColorPatchModel {
	return []extui.HighlightColorPatchModel{style.Normal, style.Selected, style.Cursor, style.SelectedCursor}
}
