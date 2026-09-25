package panel

import (
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// ShowGroupMenu changes grouping without changing the selected sort mode.
func (fp *FileSystemPanel) ShowGroupMenu() {
	if fp == nil || vtui.FrameManager == nil {
		return
	}
	menu := vtui.NewVMenu(i18n.Msg("Group.Menu"))
	visibleModes := make([]GroupModeInfo, 0, len(GroupModes))
	selected := 0
	for _, spec := range GroupModes {
		// The TUI does not run the Qt thumbnail metadata reader and therefore
		// cannot populate file-field groups.
		if spec.Mode == GroupFileField {
			continue
		}
		if spec.Mode == fp.GroupBy {
			selected = len(visibleModes)
		}
		visibleModes = append(visibleModes, spec)
		prefix := "  "
		if spec.Mode == fp.GroupBy {
			prefix = "✓ "
		}
		menu.AddItem(vtui.MenuItem{Text: prefix + i18n.Msg("Group.By"+spec.ID)})
	}
	for _, option := range []struct {
		key     string
		checked bool
	}{{"Group.Reverse", fp.GroupReverse}, {"Group.SeparateFolders", fp.GroupFoldersSeparately}, {"Group.Settings", false}} {
		prefix := "  "
		if option.checked {
			prefix = "✓ "
		}
		menu.AddItem(vtui.MenuItem{Text: prefix + i18n.Msg(option.key)})
	}
	menu.SetSelectPos(selected)
	menu.OnAction = func(idx int) {
		switch {
		case idx >= 0 && idx < len(visibleModes):
			fp.SetGrouping(visibleModes[idx].Mode, fp.GroupReverse, fp.GroupFoldersSeparately)
		case idx == len(visibleModes):
			fp.SetGrouping(fp.GroupBy, !fp.GroupReverse, fp.GroupFoldersSeparately)
		case idx == len(visibleModes)+1:
			fp.SetGrouping(fp.GroupBy, fp.GroupReverse, !fp.GroupFoldersSeparately)
		case idx == len(visibleModes)+2:
			RunAction("Panel.GroupSettings")
		}
		vtui.FrameManager.Redraw()
	}
	w := min(52, vtui.FrameManager.GetScreenSize())
	h := min(len(visibleModes)+5, vtui.FrameManager.GetScreenHeight())
	x := max(0, (fp.X1+fp.X2-w+1)/2)
	x = min(x, vtui.FrameManager.GetScreenSize()-w)
	y := max(0, (vtui.FrameManager.GetScreenHeight()-h)/2)
	menu.SetPosition(x, y, x+w-1, y+h-1)
	vtui.FrameManager.Push(menu)
}
