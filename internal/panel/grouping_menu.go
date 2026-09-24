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
	for _, spec := range GroupModes {
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
	menu.SetSelectPos(int(fp.GroupBy))
	menu.OnAction = func(idx int) {
		switch {
		case idx >= 0 && idx < len(GroupModes):
			fp.SetGrouping(GroupModes[idx].Mode, fp.GroupReverse, fp.GroupFoldersSeparately)
		case idx == len(GroupModes):
			fp.SetGrouping(fp.GroupBy, !fp.GroupReverse, fp.GroupFoldersSeparately)
		case idx == len(GroupModes)+1:
			fp.SetGrouping(fp.GroupBy, fp.GroupReverse, !fp.GroupFoldersSeparately)
		case idx == len(GroupModes)+2:
			RunAction("Panel.GroupSettings")
		}
		vtui.FrameManager.Redraw()
	}
	w := min(52, vtui.FrameManager.GetScreenSize())
	h := min(len(GroupModes)+5, vtui.FrameManager.GetScreenHeight())
	x := max(0, (fp.X1+fp.X2-w+1)/2)
	x = min(x, vtui.FrameManager.GetScreenSize()-w)
	y := max(0, (vtui.FrameManager.GetScreenHeight()-h)/2)
	menu.SetPosition(x, y, x+w-1, y+h-1)
	vtui.FrameManager.Push(menu)
}
