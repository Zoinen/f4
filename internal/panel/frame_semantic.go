package panel

import (
	"strings"

	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func viewModeName(mode ViewMode) string {
	switch mode {
	case ViewModeBrief:
		return "brief"
	case ViewModeDetailed:
		return "detailed"
	case ViewModeWide:
		return "wide"
	default:
		return "medium"
	}
}

func sortModeName(mode SortMode) string {
	switch mode {
	case SortExt:
		return "extension"
	case SortTime:
		return "time"
	case SortSize:
		return "size"
	case SortUnsorted:
		return "unsorted"
	default:
		return "name"
	}
}

// SemanticNode экспортирует PanelsFrame в семантическое дерево ShellModel
func (pf *PanelsFrame) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          strings.TrimSpace(pf.GetTitle()),
		Mode:           "panels",
		ActivePanel:    pf.ActiveIdx,
		ShowPanels:     pf.ShowPanels,
		ShowKeyBar:     pf.ShowKeyBar,
		TerminalBusy:   pf.IsPtyBusy(),
		TerminalActive: !pf.ShowPanels,
	}
	if !pf.ShowPanels {
		shell.Mode = "terminal"
	}

	for i, panel := range pf.Panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			shell.Panels = append(shell.Panels, fsp.SemanticPanelModel(ctx, i, i == pf.ActiveIdx))
		}
	}

	if pf.CmdLine != nil {
		shell.CommandLine = pf.CmdLine.SemanticModel(ctx)
	}
	if pf.TermView != nil {
		shell.Terminal = pf.TermView.SemanticModel(ctx)
	}
	if macro.MacroMgr != nil && macro.MacroMgr.Recording {
		shell.MacroRecording = true
	}

	return shell.ToMap()
}

func (pf *PanelsFrame) HandleSemanticAction(action map[string]any) bool {
	switch semantic.String(action["action"]) {
	case "activate_panel", "panel.activate":
		side := semantic.Int(action["side"])
		if side >= 0 && side < len(pf.Panels) {
			pf.ActiveIdx = side
			pf.LastKey = 0
			return true
		}
	case "panel_cursor", "panel.cursor":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.SetCursorIndex(semantic.Int(action["index"]))
			return true
		}
	case "panel_open", "panel.open":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			idx := semantic.Int(action["index"])
			pf.setActivePanelForAction(action)
			fsp.SetCursorIndex(idx)
			return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
		}
	case "panel_toggle_selection", "panel.toggleSelection":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.ToggleSelection(semantic.Int(action["index"]))
			return true
		}
	case "panel_refresh", "panel.refresh":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.ReadDirectory()
			return true
		}
	case "submit_command", "command.submit":
		if text := semantic.String(action["text"]); text != "" && pf.CmdLine != nil {
			pf.CmdLine.Edit.SetText(text)
		}
		return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "set_command_text", "command.setText":
		if pf.CmdLine != nil {
			pf.CmdLine.Edit.SetText(semantic.String(action["text"]))
			return true
		}
	case "emit_command", "command.emit":
		return vtui.FrameManager.EmitCommand(semantic.Int(action["command"]), action["args"])
	}
	return false
}

func (pf *PanelsFrame) setActivePanelForAction(action map[string]any) {
	side := semantic.Int(action["side"])
	if side >= 0 && side < len(pf.Panels) {
		pf.ActiveIdx = side
	}
}

func (pf *PanelsFrame) panelForSemanticAction(action map[string]any) *FileSystemPanel {
	side := semantic.Int(action["side"])
	if side < 0 || side >= len(pf.Panels) {
		side = pf.ActiveIdx
	}
	if fsp, ok := pf.Panels[side].(*FileSystemPanel); ok {
		return fsp
	}
	return nil
}

func (fp *FileSystemPanel) SemanticPanelModel(ctx *vtui.SemanticContext, side int, active bool) extui.PanelModel {
	var entries []extui.FileEntryModel
	selectedCount := 0
	var selectedSize int64
	var totalSize int64
	for i, entry := range fp.Entries {
		if !entry.IsDir {
			totalSize += entry.Size
		}
		if entry.Selected {
			selectedCount++
			selectedSize += entry.Size
		}
		entries = append(entries, extui.FileEntryModel{
			Index:          i,
			Name:           entry.Name,
			Size:           entry.Size,
			SizeText:       entrySizeText(entry),
			IsDir:          entry.IsDir,
			IsUp:           entry.Name == "..",
			IsHidden:       entry.IsHidden,
			IsExecutable:   entry.IsExecutable,
			IsCached:       entry.IsCached,
			Selected:       entry.Selected,
			SizeCalculated: entry.SizeCalculated,
			MTime:          entry.MTime.Format("2006-01-02 15:04"),
			Mode:           entry.Mode,
		})
	}

	groups := make([]extui.PanelGroupModel, len(fp.Groups()))
	for i, g := range fp.Groups() {
		groups[i] = extui.PanelGroupModel{Key: g.Key, Title: g.Title, StartIndex: g.StartIndex, Count: g.Count}
	}
	return extui.PanelModel{
		GroupBy: GroupModes[ValidGroupMode(fp.GroupBy)].ID, GroupReverse: fp.GroupReverse, GroupFoldersSeparately: fp.GroupFoldersSeparately,
		DisplayTop: fp.Table.TopPos, Groups: groups,
		ID:            vtui.SemanticID(fp),
		Side:          side,
		Active:        active,
		Path:          fp.Vfs.GetPath(),
		Title:         fp.Frame.GetTitle(),
		ViewMode:      viewModeName(fp.EffectiveViewMode()),
		SortMode:      sortModeName(fp.SortMode),
		SortReverse:   fp.SortReverse,
		Cursor:        fp.GetCursorIndex(),
		Top:           fp.FileTop(),
		Loading:       fp.IsLoading,
		FastFind:      fp.FastFindMode,
		FastFindText:  fp.FastFindStr,
		SelectedCount: selectedCount,
		SelectedSize:  selectedSize,
		TotalCount:    len(fp.Entries),
		TotalSize:     totalSize,
		Entries:       entries,
	}
}
