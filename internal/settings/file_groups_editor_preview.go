package settings

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/extui"
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

// Refreshed on valid draft edits, never while painting. This passive control
// stays outside the scrolling property page in both frontends.
type fileGroupEditorSample struct {
	*vtui.Group
	preview fileGroupPreview
	catalog string
	problem string
}

func newFileGroupEditorSample() *fileGroupEditorSample {
	s := &fileGroupEditorSample{Group: vtui.NewGroup(0, 0, 1, 1)}
	s.SetId("filegroups:editor-preview")
	s.SetDisabled(true)
	return s
}

func (s *fileGroupEditorSample) Show(scr *vtui.ScreenBuf) {
	width := max(1, (s.X2-s.X1+1)/4)
	for i, label := range fileGroupStateLabels() {
		x := s.X1 + i*width
		scr.Write(x, s.Y1, vtui.StringToCharInfo(vtui.TruncateString(label, width-1, "…"),
			vtui.Palette[vtui.ColDialogText]))
		name := s.preview.style.Marker + s.preview.item.Name
		text := vtui.TruncateString(name, width-1, "…")
		text += strings.Repeat(" ", max(0, width-1-vtui.StringWidth(text)))
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
	n["items"] = fileGroupStateLabels()
	n["text"] = s.problem
	return n
}

func fileGroupColourPatches(style extui.HighlightStyleModel) []extui.HighlightColorPatchModel {
	return []extui.HighlightColorPatchModel{style.Normal, style.Selected, style.Cursor, style.SelectedCursor}
}

func (s *fileGroupEditorSample) update(preview fileGroupPreview) {
	// A mini panel needs the base palette too, not only the group's patches.
	patches := make([]extui.HighlightColorPatchModel, 4)
	for state, attr := range preview.attrs {
		fg, bg := theme.GetColorRGBBoth(attr)
		patches[state] = extui.HighlightColorPatchModel{
			Foreground: fmt.Sprintf("#%06X", fg), Background: fmt.Sprintf("#%06X", bg),
		}
	}
	preview.style.Normal, preview.style.Selected = patches[0], patches[1]
	preview.style.Cursor, preview.style.SelectedCursor = patches[2], patches[3]
	s.preview = preview
}
