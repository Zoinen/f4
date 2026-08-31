package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// SemanticNode экспортирует PanelsFrame в семантическое дерево ShellModel
func (pf *PanelsFrame) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          strings.TrimSpace(pf.GetTitle()),
		Mode:           "panels",
		ActivePanel:    pf.activeIdx,
		ShowPanels:     pf.showPanels,
		ShowLeftPanel:  pf.showLeftPanel,
		ShowRightPanel: pf.showRightPanel,
		Wide:           pf.wide,
		WidePanel:      pf.widePanel,
		PanelLayout:    pf.semanticPanelLayoutModel(ctx),
		ShowKeyBar:     pf.showKeyBar,
		TerminalBusy:   pf.isPtyBusy(),
		TerminalActive: !pf.showPanels,
	}
	if !pf.showPanels {
		shell.Mode = "terminal"
	}

	for i, panel := range pf.panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			shell.Panels = append(shell.Panels, fsp.semanticPanelModel(ctx, i, i == pf.activeIdx))
		}
		if info, ok := pf.altPanels[i].(*InfoPanel); ok {
			shell.InfoPanels = append(shell.InfoPanels, info.semanticModel(i, i == pf.activeIdx))
		}
		if quick, ok := pf.altPanels[i].(*QuickViewPanel); ok {
			sourceSide := -1
			for candidate, source := range pf.panels {
				if source == quick.Source() {
					sourceSide = candidate
					break
				}
			}
			shell.QuickViews = append(shell.QuickViews,
				quick.semanticModel(i, sourceSide, i == pf.activeIdx))
		}
	}

	if pf.cmdLine != nil {
		shell.CommandLine = pf.cmdLine.semanticModel(ctx)
	}
	if pf.termView != nil {
		shell.Terminal = pf.termView.semanticModel(ctx)
	}
	if MacroMgr != nil && MacroMgr.Recording {
		shell.MacroRecording = true
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		shell.Fallback = true
		shell.FallbackReason = reason
	}

	return shell.ToMap()
}

func (pf *PanelsFrame) semanticPanelLayoutModel(ctx *vtui.SemanticContext) extui.PanelLayoutModel {
	columns := pf.lastW
	if ctx != nil && ctx.Width > 0 {
		columns = ctx.Width
	}
	if columns < 0 {
		columns = 0
	}

	widthDecrement := pf.widthDecrement
	if maxWD := (columns / 2) - 10; maxWD > 0 {
		if widthDecrement > maxWD {
			widthDecrement = maxWD
		}
		if widthDecrement < -maxWD {
			widthDecrement = -maxWD
		}
	} else {
		widthDecrement = 0
	}
	splitColumn := 0
	if columns > 0 {
		splitColumn = columns/2 - widthDecrement
	}

	clampBottomInset := func(value int) int {
		if value < 0 {
			return 0
		}
		height := pf.lastH
		if ctx != nil && ctx.Height > 0 {
			height = ctx.Height
		}
		if maxHD := height - 7; maxHD > 0 && value > maxHD {
			return maxHD
		}
		return value
	}
	return extui.PanelLayoutModel{
		Columns:              columns,
		SplitColumn:          splitColumn,
		LeftBottomInsetRows:  clampBottomInset(pf.leftHeightDecrement),
		RightBottomInsetRows: clampBottomInset(pf.rightHeightDecrement),
	}
}

func (pf *PanelsFrame) semanticGridFallbackReason() string {
	for _, panel := range pf.altPanels {
		if panel != nil && panel.Kind() != "info" && panel.Kind() != "quick_view" {
			return "the QML presentation does not yet support the " + panel.Kind() + " panel"
		}
	}
	// Horizontal split geometry is part of the bounded shell contract and is
	// rendered natively. Shortened panels expose live terminal rows, which need
	// their own incremental terminal patch before that layout can leave the
	// compatibility grid without regressing correctness.
	if pf.showPanels && (pf.leftHeightDecrement != 0 || pf.rightHeightDecrement != 0) {
		return "the QML presentation does not yet support shortened-panel layouts"
	}
	return ""
}

func (ip *InfoPanel) semanticModel(side int, active bool) extui.InfoPanelModel {
	model := extui.InfoPanelModel{
		ID:         vtui.SemanticID(ip),
		Side:       side,
		Active:     active,
		Title:      Msg("InfoPanel.Title"),
		BottomHint: Msg("InfoPanel.UnitsHint"),
	}
	row := func(label, value string) {
		model.Rows = append(model.Rows, extui.InfoPanelRowModel{Kind: "row", Label: label, Value: value})
	}
	section := func(title string) {
		model.Rows = append(model.Rows, extui.InfoPanelRowModel{Kind: "section", Label: title})
	}
	blank := func() {
		model.Rows = append(model.Rows, extui.InfoPanelRowModel{Kind: "blank"})
	}

	hostname, _ := os.Hostname()
	username := ""
	if current, err := user.Current(); err == nil {
		username = shortUsername(current.Username)
	}
	row(Msg("InfoPanel.Computer"), hostname)
	row(Msg("InfoPanel.User"), username)
	blank()

	path := ""
	if ip.src != nil && ip.src.vfs != nil {
		path = ip.src.vfs.GetPath()
	}
	fsTitle := Msg("InfoPanel.FilesystemTitle")
	if fs, ok := fsInfo(path); ok {
		if fs.Type != "" {
			fsTitle = fmt.Sprintf("%s (%s)", fsTitle, fs.Type)
		}
		section(fsTitle)
		row(Msg("InfoPanel.Total"), formatBytes(fs.Total))
		row(Msg("InfoPanel.Free"), formatBytes(fs.Free))
		if fs.Label != "" {
			row(Msg("InfoPanel.Label"), fs.Label)
		}
		if fs.Serial != "" {
			row(Msg("InfoPanel.Serial"), fs.Serial)
		}
		row(Msg("InfoPanel.CurrentDir"), path)
		if fs.Mount != "" && fs.Mount != path {
			row(Msg("InfoPanel.Mount"), fs.Mount)
		}
		if fs.MaxFilename > 0 {
			row(Msg("InfoPanel.MaxFilename"), fmt.Sprintf("%d", fs.MaxFilename))
		}
		if fs.Flags != "" {
			row(Msg("InfoPanel.Flags"), fs.Flags)
		}
	} else {
		section(fsTitle)
		row(Msg("InfoPanel.CurrentDir"), path)
	}

	if mem, ok := memInfo(); ok {
		blank()
		section(Msg("InfoPanel.MemoryTitle"))
		row(Msg("InfoPanel.MemLoad"), fmt.Sprintf("%d%%", mem.LoadPercent))
		row(Msg("InfoPanel.MemTotal"), formatBytes(mem.Total))
		row(Msg("InfoPanel.MemFree"), formatBytes(mem.Free))
		if mem.Shared > 0 {
			row(Msg("InfoPanel.MemShared"), formatBytes(mem.Shared))
		}
		if mem.Buffered > 0 {
			row(Msg("InfoPanel.MemBuffered"), formatBytes(mem.Buffered))
		}
		if mem.SwapTotal > 0 {
			row(Msg("InfoPanel.SwapTotal"), formatBytes(mem.SwapTotal))
			row(Msg("InfoPanel.SwapFree"), formatBytes(mem.SwapFree))
		}
	}
	return model
}

// HandleSemanticAction обрабатывает нативные GUI-действия для ViewerView
func (vv *ViewerView) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if vtui.SemanticID(vv) != target {
		return false
	}

	switch semanticString(action["action"]) {
	case "viewer.scroll":
		generation, accepted := semanticAcceptWindowGeneration(action,
			vv.semanticWindowGeneration, &vv.semanticWindowRequestGeneration)
		if !accepted {
			return true
		}
		offset := semanticInt64(action["offset"])
		if offset < 0 {
			offset = 0
		}
		if offset > vv.backend.Size() {
			offset = vv.backend.Size()
		}
		if vv.HexMode {
			offset &= ^int64(0xF)
		} else {
			offset = vv.clampTextScrollOffset(offset)
			offset = vv.backend.FindLineStart(offset)
		}
		vv.TopOffset = offset
		vv.eofVisible = false
		vv.semanticPendingScroll = false
		vv.semanticPendingGeneration = 0
		vv.semanticWindowGeneration = generation
		return true
	case "viewer.scrollWindow":
		generation, accepted := semanticAcceptWindowGeneration(action,
			vv.semanticWindowGeneration, &vv.semanticWindowRequestGeneration)
		if !accepted {
			return true
		}
		offset := semanticInt64(action["offset"])
		offset = max(int64(0), min(offset, vv.backend.Size()))
		if vv.HexMode {
			vv.TopOffset = offset &^ int64(0xF)
			vv.semanticPendingScroll = false
			vv.semanticPendingGeneration = 0
			vv.semanticWindowGeneration = generation
			return true
		}
		offset = vv.clampTextScrollOffset(offset)
		resolved, ready := vv.semanticResolveTextWindowOffset(offset)
		if !ready {
			vv.semanticPendingScroll = true
			vv.semanticPendingOffset = offset
			vv.semanticPendingGeneration = generation
			return true
		}
		vv.TopOffset = resolved
		vv.eofVisible = false
		vv.semanticPendingScroll = false
		vv.semanticPendingGeneration = 0
		vv.semanticWindowGeneration = generation
		return true
	case "control.focus":
		vv.SetFocus(true)
		return true
	}
	return false
}

// HandleSemanticAction глобально маршрутизирует семантические действия из внешнего GUI
func HandleSemanticAction(action map[string]any) bool {
	if action == nil {
		return false
	}
	actionName := semanticString(action["action"])
	target := semanticString(action["target"])
	if actionName == "toast.dismiss" {
		return vtui.FrameManager.HandleSemanticAction(action)
	}
	if target == "app" && (actionName == "presentation.toggle" || actionName == "toggle_presentation") {
		toggleGuiPresentation()
		return true
	}
	if handled, claimed := handleOperationsQueueWorkspaceClose(action); claimed {
		return handled
	}
	if strings.HasPrefix(actionName, "workspace.") || actionName == "tab.activate" || strings.HasPrefix(target, "workspace-") {
		return vtui.FrameManager.HandleSemanticAction(action)
	}
	if kind, _ := action["kind"].(string); kind == "command" {
		return vtui.FrameManager.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	if actionName == "menuBar.itemSelect" || actionName == "menuBar.itemActivate" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			menuIndex := semanticInt(action["menuIndex"])
			itemIndex := semanticInt(action["index"])
			if menuIndex >= 0 && menuIndex < len(mb.Items) &&
				itemIndex >= 0 && itemIndex < len(mb.Items[menuIndex].SubItems) &&
				!mb.Items[menuIndex].SubItems[itemIndex].Separator {
				if actionName == "menuBar.itemSelect" {
					// A pointer-hover action is deliberately non-activating. It may
					// arrive after the pointer has already moved to another top-level
					// menu, so accepting it must never reopen the stale submenu.
					if !mb.Active || mb.SelectPos != menuIndex {
						return false
					}
					menu := activeMenuBarSubmenu(mb, semanticString(action["target"]))
					if menu == nil {
						return false
					}
					menu.SetSelectPos(itemIndex)
					vtui.FrameManager.DeclareSemanticMenuState()
					return true
				}

				// Click activation is atomic and may legitimately target a submenu
				// which QML was previewing before Go had materialized it.
				mb.Active = true
				if mb.SelectPos != menuIndex {
					mb.ActivateSubMenu(menuIndex)
				}
				if menu := activeMenuBarSubmenu(mb, ""); menu != nil {
					menu.SetSelectPos(itemIndex)
					handled := menu.ProcessKey(&vtinput.InputEvent{
						Type:           vtinput.KeyEventType,
						KeyDown:        true,
						VirtualKeyCode: vtinput.VK_RETURN,
						InputSource:    "qt_semantic",
					})
					if handled {
						// Semantic actions execute outside FrameManager's normal
						// keyboard-dispatch loop. Complete the same post-dispatch
						// lifecycle here: close the menu bar and remove every frame
						// whose command marked it done (for example viewer/editor
						// after File > Exit).
						vtui.FrameManager.EmitCommand(vtui.CmMenuClose, nil)
						frames := append([]vtui.Frame(nil),
							vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)...)
						for i := len(frames) - 1; i >= 0; i-- {
							if frames[i].IsDone() && frames[i].GetType() == vtui.TypeMenu {
								vtui.FrameManager.RemoveFrame(frames[i])
							}
						}

						frames = append(frames[:0],
							vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)...)
						closeWholeScreen := len(vtui.FrameManager.Screens) > 1
						hasDoneFrame := false
						for _, frame := range frames {
							if frame.IsDone() {
								hasDoneFrame = true
								continue
							}
							if frame.GetType() != vtui.TypeDesktop {
								closeWholeScreen = false
							}
						}
						if hasDoneFrame && closeWholeScreen {
							// Viewer/editor live on their own vtui screen. Closing
							// that screen restores the previous commander panels;
							// merely removing the document frame would expose its
							// otherwise-empty Desktop fallback.
							vtui.FrameManager.CloseActiveScreen()
						} else {
							for i := len(frames) - 1; i >= 0; i-- {
								if frames[i].IsDone() {
									vtui.FrameManager.RemoveFrame(frames[i])
								}
							}
						}
						vtui.FrameManager.Redraw()
					}
					return handled
				}
			}
		}
		return false
	}
	if actionName == "menu_bar_activate" || actionName == "menuBar.activate" || actionName == "menuBar.toggle" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			idx := semanticInt(action["index"])
			if idx >= 0 && idx < len(mb.Items) {
				if actionName == "menuBar.toggle" && mb.Active && mb.SelectPos == idx {
					frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
					for i := len(frames) - 1; i >= 0; i-- {
						if menu, _ := appFrameVMenu(frames[i]); menu != nil {
							frames[i].Close()
							break
						}
					}
					mb.Active = false
					vtui.FrameManager.DeclareSemanticMenuState()
					return true
				}
				mb.Active = true
				mb.ActivateSubMenu(idx)
				return true
			}
		}
	}

	activeIdx := vtui.FrameManager.ActiveIdx
	frames := vtui.FrameManager.GetActiveFrames(activeIdx)
	// Command-line autocomplete is a modal frame in the terminal frontend, but
	// its native QML popup keeps the selection locally and submits an explicit
	// semantic action. Route that action straight to the owning PanelsFrame:
	// otherwise the modal overlay may consume it before the shell ever writes
	// the command to its PTY.
	if actionName == "command.submit" || actionName == "submit_command" ||
		actionName == "command.complete" || actionName == "complete_command" {
		for i := len(frames) - 1; i >= 0; i-- {
			if panels, ok := frames[i].(*PanelsFrame); ok {
				if panels.HandleSemanticAction(action) {
					if !vtui.FrameManager.CurrentTaskDeclaredUnchanged() {
						vtui.FrameManager.Redraw()
					}
					return true
				}
			}
		}
	}
	for i := len(frames) - 1; i >= 0; i-- {
		if h, ok := frames[i].(vtui.SemanticActionHandler); ok && h.HandleSemanticAction(action) {
			if !vtui.FrameManager.CurrentTaskDeclaredUnchanged() {
				vtui.FrameManager.Redraw()
			}
			return true
		}
	}

	target = semanticString(action["target"])
	if target == "" {
		return false
	}
	for i := len(frames) - 1; i >= 0; i-- {
		if handleSemanticFrameAction(frames[i], target, action) {
			if !semanticMenuPresentationAction(actionName) {
				vtui.FrameManager.Redraw()
			}
			return true
		}
	}
	return false
}

func semanticMenuPresentationAction(actionName string) bool {
	switch actionName {
	case "menu_select", "menu.select", "menu_scroll", "menu.scroll",
		"menuBar.itemSelect", "menu_bar_activate", "menuBar.activate",
		"menuBar.toggle":
		return true
	default:
		return false
	}
}

func activeMenuBarSubmenu(mb *vtui.MenuBar, target string) *vtui.VMenu {
	if vtui.FrameManager == nil || mb == nil {
		return nil
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for i := len(frames) - 1; i >= 0; i-- {
		menu, _ := appFrameVMenu(frames[i])
		if menu == nil || !appIsMenuBarSubmenu(frames[i], mb) {
			continue
		}
		if target != "" && vtui.SemanticID(frames[i]) != target {
			continue
		}
		return menu
	}
	return nil
}

func toggleGuiPresentation() GuiPresentationMode {
	AppConfig.GuiPresentation = nextGuiPresentationMode(AppConfig.GuiPresentation)
	SaveConfig()
	if vtui.FrameManager != nil {
		vtui.FrameManager.HardRefresh()
		message := Msg("Presentation.GUI")
		if AppConfig.GuiPresentation == GuiPresentationText {
			message = Msg("Presentation.Text")
		}
		vtui.ShowToast(message, 2*time.Second)
	}
	return AppConfig.GuiPresentation
}

func handleSemanticFrameAction(frame vtui.Frame, target string, action map[string]any) bool {
	if vtui.SemanticID(frame) == target {
		switch semanticString(action["action"]) {
		case "close", "dialog.close", "window.close":
			frame.Close()
			return true
		case "menu_activate", "menu.activate":
			if menu, _ := appFrameVMenu(frame); menu != nil {
				idx := semanticInt(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator {
					menu.SetSelectPos(idx)
					return menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		case "menu_select", "menu.select":
			if menu, _ := appFrameVMenu(frame); menu != nil {
				idx := semanticInt(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator {
					menu.SetSelectPos(idx)
					vtui.FrameManager.DeclareSemanticMenuState()
					return true
				}
			}
		case "menu_scroll", "menu.scroll":
			if menu, _ := appFrameVMenu(frame); menu != nil {
				menu.ScrollBy(semanticInt(action["delta"]))
				vtui.FrameManager.DeclareSemanticMenuState()
				return true
			}
		}
	}
	if c, ok := frame.(vtui.Container); ok {
		return handleSemanticChildrenAction(c.GetChildren(), target, action)
	}
	return false
}

func handleSemanticChildrenAction(children []vtui.UIElement, target string, action map[string]any) bool {
	for _, child := range children {
		if vtui.SemanticID(child) == target {
			return handleSemanticElementAction(child, action)
		}
		if c, ok := child.(vtui.Container); ok {
			if handleSemanticChildrenAction(c.GetChildren(), target, action) {
				return true
			}
		}
	}
	return false
}

func handleSemanticElementAction(el vtui.UIElement, action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "focus", "control.focus":
		el.SetFocus(true)
		return true
	case "activate", "control.activate":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "toggle", "control.toggle":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE, Char: ' ', InputSource: "qt_semantic"})
	case "set_text", "control.setText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.SetText(semanticString(action["text"]))
			if edit.OnTextChange != nil {
				edit.OnTextChange(edit.GetText())
			}
			return true
		}
	case "insert_text", "control.insertText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.InsertString(semanticString(action["text"]))
			return true
		}
	case "select", "control.select":
		idx := semanticInt(action["index"])
		switch w := el.(type) {
		case *vtui.RadioGroup:
			if idx >= 0 && idx < len(w.Items) {
				w.SetData(idx)
				return true
			}
		case *vtui.ListBox:
			if idx >= 0 && idx < len(w.Items) {
				w.SetSelectPos(idx)
				return true
			}
		case *vtui.ComboBox:
			if idx >= 0 && idx < len(w.Menu.Items) {
				w.Menu.SetSelectPos(idx)
				w.Edit.SetText(w.Menu.Items[idx].Text)
				return true
			}
		}
	}
	return false
}

func (pf *PanelsFrame) HandleSemanticAction(action map[string]any) bool {
	if semanticString(action["action"]) == "quickView.scroll" {
		for _, panel := range pf.altPanels {
			if quick, ok := panel.(*QuickViewPanel); ok && quick.HandleSemanticAction(action) {
				return true
			}
		}
		return false
	}
	switch semanticString(action["action"]) {
	case "activate_panel", "panel.activate":
		side := semanticInt(action["side"])
		if side >= 0 && side < len(pf.panels) {
			pf.setActivePanelForAction(action)
			pf.lastKey = 0
			if fsp, ok := pf.panels[side].(*FileSystemPanel); ok {
				fsp.clearFastFindForSemanticPointerIntent()
			}
			return true
		}
	case "panel_navigate_path", "panel.navigatePath":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil || fsp.vfs == nil {
			return false
		}
		if benchmark := navigationBenchmarkCurrentUI(); benchmark != nil {
			benchmark.setSide(pf.panelIndexForSemanticAction(action))
		}
		target := strings.TrimSpace(semanticString(action["path"]))
		if target == "" {
			return false
		}
		oldPath := fsp.vfs.GetPath()
		if target == oldPath {
			return true
		}
		if err := fsp.setKnownDirectoryPath(target); err != nil {
			fsp.showDirectoryError(" Error ", fmt.Sprintf("Cannot access folder:\n%v", err))
			return true
		}
		pf.setActivePanelForAction(action)
		fsp.clearFastFindForSemanticPointerIntent()
		fsp.pendingSelection = fsp.vfs.Base(oldPath)
		fsp.ReadDirectory()
		return true
	case "panel_drive_menu", "panel.driveMenu":
		side := pf.panelIndexForSemanticAction(action)
		fsp := pf.panelForSemanticAction(action)
		if side < 0 || fsp == nil {
			return false
		}
		pf.setActivePanelForAction(action)
		pf.lastKey = 0
		fsp.clearFastFindForSemanticPointerIntent()
		pf.showDriveMenu(side)
		return true
	case "panel_cursor", "panel.cursor":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			// A native GUI row click targets an item and activates its panel as
			// one visual operation. Applying those intents atomically prevents
			// an intermediate scene from highlighting the panel's old cursor.
			if semanticBool(action["activate"]) {
				pf.setActivePanelForAction(action)
				pf.lastKey = 0
			}
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetCursorIndex(idx)
			return true
		}
	case "panel_pointer", "panel.pointer":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			phase := semanticString(action["phase"])
			button := semanticString(action["button"])
			if phase == "up" || phase == "cancel" {
				fsp.semanticPointerRelease()
				return true
			}
			if phase == "click" {
				// The click is intentionally represented in the semantic stream,
				// while all immediate state changes happen on mouse-down just as
				// they do in the terminal frontend.
				return true
			}
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			if phase == "down" || phase == "doubleClick" {
				pf.setActivePanelForAction(action)
				pf.lastKey = 0
				fsp.clearFastFindForSemanticPointerIntent()
			}
			switch button {
			case "right":
				if phase == "down" {
					return fsp.semanticRightPointerDown(idx)
				}
				if phase == "move" {
					return fsp.semanticRightPointerMove(idx)
				}
				if phase == "doubleClick" {
					return fsp.semanticRightPointerDoubleClick(idx)
				}
			case "middle":
				if phase == "down" {
					fsp.SetCursorIndex(idx)
					return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			case "left":
				if phase == "down" || phase == "move" {
					fsp.SetCursorIndex(idx)
					return true
				}
				if phase == "doubleClick" {
					fsp.SetCursorIndex(idx)
					return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		}
	case "panel_open", "panel.open":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			if benchmark := navigationBenchmarkCurrentUI(); benchmark != nil {
				benchmark.setSide(pf.panelIndexForSemanticAction(action))
			}
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			pf.setActivePanelForAction(action)
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetCursorIndex(idx)
			return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
		}
	case "panel_toggle_selection", "panel.toggleSelection":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return false
			}
			if rawRevision, present := action["selectionRevision"]; present && semanticInt64(rawRevision) != fsp.selectionRevision {
				return false
			}
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.ToggleSelection(idx)
			return true
		}
	case "panel_set_selection", "panel.setSelection":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			if !fsp.applySemanticSelection(action) {
				return false
			}
			if semanticBool(action["activate"]) {
				pf.setActivePanelForAction(action)
				pf.lastKey = 0
			}
			fsp.clearFastFindForSemanticPointerIntent()
			return true
		}
	case "panel_set_wide", "panel.setWide":
		side := pf.panelIndexForSemanticAction(action)
		if side < 0 {
			return false
		}
		enabled, present := action["enabled"]
		if !present {
			return false
		}
		if semanticBool(enabled) {
			pf.setWidePanel(side)
		} else if pf.wide && pf.widePanel == side {
			pf.exitWide()
		} else {
			return false
		}
		return true
	case "panel_set_gallery_layout", "panel.setGalleryLayout":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semanticString(action["layoutMode"])
		if modeValue == "" {
			modeValue = semanticString(action["galleryLayoutMode"])
		}
		mode, ok := parseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		columns := semanticInt(action["columnCount"])
		if columns == 0 {
			columns = fsp.effectiveGalleryColumnCount()
		}
		previousRevision := fsp.galleryLayoutRevision
		if !fsp.SetGalleryLayout(mode, columns) {
			return false
		}
		pf.updateMenuCheckmarks()
		if fsp.galleryLayoutRevision != previousRevision {
			persistNativePanelLayoutSession(pf)
		}
		return true
	case "panel_set_gallery_density", "panel.setGalleryDensity":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semanticString(action["layoutMode"])
		if modeValue == "" {
			modeValue = semanticString(action["galleryLayoutMode"])
		}
		if modeValue == "" {
			modeValue = string(fsp.effectiveGalleryLayoutMode())
		}
		mode, ok := parseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		previousRevision := fsp.galleryLayoutRevision
		if !fsp.SetGalleryDensity(mode, semanticInt(action["density"])) {
			return false
		}
		if fsp.galleryLayoutRevision != previousRevision {
			persistNativePanelLayoutSession(pf)
		}
		return true
	case "panel_reset_gallery_density", "panel.resetGalleryDensity":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semanticString(action["layoutMode"])
		if modeValue == "" {
			modeValue = semanticString(action["galleryLayoutMode"])
		}
		if modeValue == "" {
			modeValue = string(fsp.effectiveGalleryLayoutMode())
		}
		mode, ok := parseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		previousRevision := fsp.galleryLayoutRevision
		if !fsp.ResetGalleryDensity(mode) {
			return false
		}
		if fsp.galleryLayoutRevision != previousRevision {
			persistNativePanelLayoutSession(pf)
		}
		return true
	case "panel_sort", "panel.sort":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			mode, ok := parseSortModeName(semanticString(action["mode"]))
			if !ok {
				return false
			}
			pf.setActivePanelForAction(action)
			pf.lastKey = 0
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetSortMode(mode)
			pf.updateMenuCheckmarks()
			return true
		}
	case "panel_sort_menu", "panel.sortMenu":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			pf.setActivePanelForAction(action)
			pf.lastKey = 0
			fsp.clearFastFindForSemanticPointerIntent()
			actionSortMenu(pf)
			return true
		}
	case "panel_refresh", "panel.refresh":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.ReadDirectory()
			return true
		}
	case "submit_command", "command.submit":
		if text := semanticString(action["text"]); text != "" && pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(text)
		}
		closeActiveAutocompleteMenus()
		return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "complete_command", "command.complete":
		// QML keeps autocomplete selection local so arrow-key repeats never
		// wait for a semantic-scene round trip.  Tab commits that explicit
		// selection, if any, but an untouched menu merely closes and preserves
		// the exact command line.
		if text, present := action["text"]; present && pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(semanticString(text))
		}
		closeActiveAutocompleteMenus()
		return true
	case "set_command_text", "command.setText":
		if pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(semanticString(action["text"]))
			return true
		}
	case "emit_command", "command.emit":
		return vtui.FrameManager.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	return false
}

func closeActiveAutocompleteMenus() {
	if vtui.FrameManager == nil {
		return
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		if _, ok := frame.(*vtui.AutoCompleteMenu); ok {
			frame.Close()
		}
	}
}

func (pf *PanelsFrame) setActivePanelForAction(action map[string]any) {
	side := pf.panelIndexForSemanticAction(action)
	if side >= 0 && side < len(pf.panels) {
		activeChanged := pf.activeIdx != side
		pf.activeIdx = side
		// A native panel interaction has the same input-focus semantics as a
		// terminal mouse press. In search-first mode it must return keyboard
		// input to the selected panel so the following Tab switches panels. An
		// already panel-focused active side is a true no-op and must not enqueue
		// another redraw/semantic export.
		if activeChanged || pf.commandLineFocused {
			pf.setCommandLineFocus(false)
		}
	}
}

func (pf *PanelsFrame) panelIndexForSemanticAction(action map[string]any) int {
	side := pf.activeIdx
	if _, present := action["side"]; present {
		side = semanticInt(action["side"])
	}
	if side < 0 || side >= len(pf.panels) {
		return -1
	}
	return side
}

func (pf *PanelsFrame) panelForSemanticAction(action map[string]any) *FileSystemPanel {
	side := pf.panelIndexForSemanticAction(action)
	if side < 0 {
		return nil
	}
	if fsp, ok := pf.panels[side].(*FileSystemPanel); ok {
		return fsp
	}
	return nil
}

// Native FileSystemPanel mouse presses leave fast-find before applying their
// pointer action. Gallery sends the equivalent intent through semantic
// actions, so clear both the visible mode and its stale query there as well.
func (fp *FileSystemPanel) clearFastFindForSemanticPointerIntent() {
	if !fp.fastFindMode && fp.fastFindStr == "" {
		return
	}
	fp.fastFindMode = false
	fp.fastFindStr = ""
}

func (fp *FileSystemPanel) semanticEntryIndex(action map[string]any) (idx int, ok bool) {
	benchmark := navigationBenchmarkCurrentUI()
	if benchmark != nil {
		benchmark.event("semantic_entry_lookup.begin", "go.ui",
			"entryId", semanticString(action["entryId"]), "entries", len(fp.entries))
		defer func() {
			benchmark.event("semantic_entry_lookup.end", "go.ui",
				"entryId", semanticString(action["entryId"]), "entries", len(fp.entries),
				"index", idx, "found", ok, "catalogRevision", fp.catalogRevision)
		}()
	}
	entryID := semanticString(action["entryId"])
	rawRevision, hasRevision := action["catalogRevision"]
	if extUiPanelCatalogRowsIsEnabled() && entryID != "" && hasRevision &&
		semanticInt64(rawRevision) == fp.catalogRevision {
		if rawIndex, present := action["index"]; present {
			candidate := semanticInt(rawIndex)
			if candidate >= 0 && candidate < len(fp.entries) &&
				fp.entries[candidate] != nil {
				sourceKind, _ := fp.semanticSourceInfo()
				actualID, _ := fp.semanticEntryMetadata(
					fp.entries[candidate], sourceKind)
				if actualID == entryID {
					return candidate, true
				}
			}
		}
	}
	// Native catalog actions carry a stable entry identity. When the action is
	// for the exact catalog we just exported, resolve that identity through the
	// immutable static catalog instead of fingerprinting every row again. The
	// cheap identity check against the live row makes this safe even for a
	// plugin/test which replaced or reordered fp.entries without going through a
	// normal panel mutation path; such a mismatch falls back to the exhaustive
	// revision update below.
	static := fp.semanticStaticCache
	if entryID != "" && hasRevision && static != nil &&
		static.catalogRevision == fp.catalogRevision &&
		len(static.entries) == len(fp.entries) &&
		semanticInt64(rawRevision) == fp.catalogRevision {
		if candidate, present := static.entryIndexByID[entryID]; present &&
			candidate >= 0 && candidate < len(fp.entries) {
			sourceKind, _ := fp.semanticSourceInfo()
			actualID, _ := fp.semanticEntryMetadata(fp.entries[candidate], sourceKind)
			if actualID == entryID {
				return candidate, true
			}
		}
	}

	fp.updateSemanticRevisions()
	if hasRevision && semanticInt64(rawRevision) != fp.catalogRevision {
		// A useful provisional window can be replaced by the authoritative
		// catalog while an Enter action is already in transit. Opening remains
		// safe when both the row position and its path-derived stable identity
		// still match the current catalog. Keep every other revisioned action
		// strict: stale cursor, pointer, and selection intents must never move to
		// a different row after a replacement.
		if semanticString(action["action"]) == "panel.open" && entryID != "" {
			if rawIndex, present := action["index"]; present {
				candidate := semanticInt(rawIndex)
				if candidate >= 0 && candidate < len(fp.entries) &&
					fp.entries[candidate] != nil {
					sourceKind, _ := fp.semanticSourceInfo()
					actualID, _ := fp.semanticEntryMetadata(
						fp.entries[candidate], sourceKind)
					if actualID == entryID {
						return candidate, true
					}
				}
			}
		}
		return 0, false
	}

	if entryID != "" {
		if static := fp.semanticStaticCache; static != nil &&
			static.catalogRevision == fp.catalogRevision &&
			len(static.entries) == len(fp.entries) {
			if candidate, present := static.entryIndexByID[entryID]; present {
				return candidate, true
			}
		}
		sourceKind, _ := fp.semanticSourceInfo()
		for idx, entry := range fp.entries {
			id, _ := fp.semanticEntryMetadata(entry, sourceKind)
			if id == entryID {
				return idx, true
			}
		}
		// Dropping an action is invisible to the user, so leave a trail: a
		// client whose intent is silently discarded here simply appears to
		// stop responding to clicks.
		vtui.DebugLog("SEMANTIC: dropped action, entryId %q not in %q (%d entries, catalogRevision %d)",
			entryID, fp.vfs.GetPath(), len(fp.entries), fp.catalogRevision)
		return 0, false
	}

	if _, ok := action["index"]; !ok {
		return 0, false
	}
	// Preserve the v2 index action behavior: cursor/open clamp through
	// SetCursorIndex and selection toggles outside the range are harmless.
	return semanticInt(action["index"]), true
}

func (fp *FileSystemPanel) applySemanticSelection(action map[string]any) bool {
	fp.updateSemanticRevisions()
	if rawRevision, ok := action["catalogRevision"]; ok && semanticInt64(rawRevision) != fp.catalogRevision {
		return false
	}
	if rawRevision, ok := action["selectionRevision"]; ok && semanticInt64(rawRevision) != fp.selectionRevision {
		return false
	}

	entryIDs := semanticStringSlice(action["entryIds"])
	if entryID := semanticString(action["entryId"]); entryID != "" {
		entryIDs = append(entryIDs, entryID)
	}
	wanted := make(map[int]struct{}, len(entryIDs))
	sourceKind, _ := fp.semanticSourceInfo()
	byID := make(map[string]int, len(fp.entries))
	for idx, entry := range fp.entries {
		id, _ := fp.semanticEntryMetadata(entry, sourceKind)
		byID[id] = idx
	}
	for _, id := range entryIDs {
		idx, ok := byID[id]
		if !ok {
			return false
		}
		wanted[idx] = struct{}{}
	}
	for _, idx := range semanticIntSlice(action["indices"]) {
		if idx < 0 || idx >= len(fp.entries) {
			return false
		}
		wanted[idx] = struct{}{}
	}

	selectionChanges := make(map[int]bool)
	if rawChanges, present := action["changes"]; present {
		changes, ok := semanticMapSlice(rawChanges)
		if !ok {
			return false
		}
		for _, change := range changes {
			id := semanticString(change["entryId"])
			selected, hasSelected := change["selected"]
			idx, found := byID[id]
			if id == "" || !hasSelected || !found {
				return false
			}
			selectionChanges[idx] = semanticBool(selected)
		}
	}

	cursorIndex := -1
	hasCursor := false
	if cursorID := semanticString(action["cursorEntryId"]); cursorID != "" {
		var found bool
		cursorIndex, found = byID[cursorID]
		if !found {
			return false
		}
		hasCursor = true
	} else if rawCursorIndex, present := action["cursorIndex"]; present {
		cursorIndex = semanticInt(rawCursorIndex)
		if cursorIndex < 0 || cursorIndex >= len(fp.entries) {
			return false
		}
		hasCursor = true
	}

	mode := strings.ToLower(semanticString(action["mode"]))
	if mode == "" && len(selectionChanges) > 0 {
		mode = "set"
	} else if mode == "" {
		mode = "replace"
	}
	switch mode {
	case "replace":
		for idx := range fp.entries {
			_, selected := wanted[idx]
			fp.SetItemSelected(idx, selected)
		}
	case "add":
		for idx := range wanted {
			fp.SetItemSelected(idx, true)
		}
	case "remove":
		for idx := range wanted {
			fp.SetItemSelected(idx, false)
		}
	case "toggle":
		for idx := range wanted {
			fp.ToggleSelection(idx)
		}
	case "set":
		if len(selectionChanges) == 0 {
			return false
		}
		for idx, selected := range selectionChanges {
			fp.SetItemSelected(idx, selected)
		}
	default:
		return false
	}
	if hasCursor {
		fp.SetCursorIndex(cursorIndex)
	}
	return true
}

func (fp *FileSystemPanel) semanticSourceInfo() (sourceKind string, previewCapable bool) {
	provider, ok := fp.vfs.(vfs.LocalPathProvider)
	if ok {
		if _, err := provider.LocalPath(fp.vfs.GetPath()); err == nil {
			return "local", true
		}
	}
	return "vfs", fp.vfs != nil
}

func (fp *FileSystemPanel) semanticEntryMetadata(entry *fileEntry, sourceKind string) (entryID, logicalPath string) {
	logicalPath = fp.vfs.Join(fp.vfs.GetPath(), entry.Name)
	// Stable semantic identity must not perform LocalPath resolution.  Besides
	// being potentially expensive for remote providers, local materialization
	// is deferred metadata and must not advance the base catalog revision.
	identity := sourceKind + "\x00" + fmt.Sprintf("%T", fp.vfs) + "\x00" + logicalPath
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("entry-%x", sum[:16]), logicalPath
}

func writeSemanticFingerprintString(h hash.Hash, value string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}

func semanticMTimeNanos(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixNano()
}

func semanticWriteUint64(h hash.Hash, value uint64) {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], value)
	_, _ = h.Write(encoded[:])
}

func semanticImageExtensions() map[string]struct{} {
	// Snapshotting the decoder registry is extension-only: it
	// never opens a file, invokes a decoder, or probes a plugin.  This gives Qt
	// an explicit answer in the first catalog and prevents synchronous format
	// discovery while delegates are being created.
	extensions := make(map[string]struct{})
	for _, decoder := range allImageDecoders() {
		for _, extension := range decoder.Extensions {
			extension = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
			if extension != "" {
				extensions[extension] = struct{}{}
			}
		}
	}
	return extensions
}

func semanticEntryIsImage(entry *fileEntry, extensions map[string]struct{}) bool {
	if entry == nil || entry.IsDir || entry.Name == ".." {
		return false
	}
	_, ok := extensions[imageExtension(entry.Name)]
	return ok
}

func semanticHighlighterRevision() int64 {
	if GlobalFileHighlighter == nil {
		return 0
	}
	return GlobalFileHighlighter.Revision
}

// semanticHighlighterTimeDependencies reports the timestamp fields which can
// affect the currently configured highlight result. MTime is always part of
// deferred metadata, but ATime/CTime are not: hashing them unconditionally
// makes an ordinary Windows directory traversal look like a semantic metadata
// change because the OS updates the parent directory's access time. Keep those
// otherwise-invisible timestamps authoritative only when a rule actually uses
// them.
func semanticHighlighterTimeDependencies() (accessed, created bool) {
	if GlobalFileHighlighter == nil {
		return false, false
	}
	for _, rule := range GlobalFileHighlighter.Rules {
		usesDate := !rule.DateAfter.IsZero() || rule.DateAfterDur > 0 ||
			!rule.DateBefore.IsZero() || rule.DateBeforeDur > 0
		if !usesDate {
			continue
		}
		switch rule.DateType {
		case DateAccessed:
			accessed = true
		case DateCreated:
			created = true
		}
	}
	return accessed, created
}

func (fp *FileSystemPanel) semanticFingerprintsForEntries(entries []*fileEntry) (uint64, uint64, uint64) {
	catalog := fnv.New64a()
	metadata := fnv.New64a()
	selection := fnv.New64a()
	selectedEntryIDs := make([]string, 0)
	sourceKind, previewCapable := fp.semanticSourceInfo()
	imageExtensions := semanticImageExtensions()
	highlightUsesATime, highlightUsesCTime := semanticHighlighterTimeDependencies()
	writeSemanticFingerprintString(catalog, sourceKind)
	writeSemanticFingerprintString(catalog, fp.vfs.GetPath())
	writeSemanticFingerprintString(metadata, sourceKind)
	writeSemanticFingerprintString(metadata, fp.vfs.GetPath())
	semanticWriteUint64(metadata, uint64(semanticHighlighterRevision()))
	// This presentation option changes the normalized display fields consumed
	// by unified Columns/Details without changing any VFS entry. Treat it as a
	// catalog change so persistent Gallery sessions receive the new appearance
	// instead of retaining their previous name split indefinitely.
	if AppConfig.SeparateFileExtensions {
		_, _ = catalog.Write([]byte{1})
	} else {
		_, _ = catalog.Write([]byte{0})
	}
	if previewCapable {
		_, _ = catalog.Write([]byte{1})
	} else {
		_, _ = catalog.Write([]byte{0})
	}
	for _, entry := range entries {
		entryID, logicalPath := fp.semanticEntryMetadata(entry, sourceKind)
		writeSemanticFingerprintString(catalog, entryID)
		writeSemanticFingerprintString(catalog, logicalPath)
		writeSemanticFingerprintString(catalog, entry.Name)
		baseFlags := byte(0)
		if entry.IsDir {
			baseFlags |= 1 << 0
		}
		if entry.Name == ".." {
			baseFlags |= 1 << 1
		}
		if entry.NoExtension {
			baseFlags |= 1 << 2
		}
		if semanticEntryIsImage(entry, imageExtensions) {
			baseFlags |= 1 << 3
		}
		if entry.IsHidden {
			baseFlags |= 1 << 4
		}
		_, _ = catalog.Write([]byte{baseFlags})

		writeSemanticFingerprintString(metadata, entryID)
		writeSemanticFingerprintString(metadata, entry.Mode)
		// Provider revisions describe file content, not row identity/layout.
		// Keep them out of CatalogRevision while still invalidating deferred
		// metadata and broker-backed derived artifacts when bytes change.
		writeSemanticFingerprintString(metadata, entry.Revision)
		semanticWriteUint64(metadata, uint64(entry.Size))
		semanticWriteUint64(metadata, uint64(semanticMTimeNanos(entry.MTime)))
		if highlightUsesATime {
			semanticWriteUint64(metadata, uint64(semanticMTimeNanos(entry.ATime)))
		}
		if highlightUsesCTime {
			semanticWriteUint64(metadata, uint64(semanticMTimeNanos(entry.CTime)))
		}
		// Highlight rules can derive read-only/system/archive state from these
		// platform mode fields even though the raw values are not serialized.
		semanticWriteUint64(metadata, uint64(entry.UnixMode))
		semanticWriteUint64(metadata, uint64(entry.WinAttrs))
		metadataFlags := byte(0)
		if entry.IsExecutable {
			metadataFlags |= 1 << 0
		}
		if entry.SizeCalculated {
			metadataFlags |= 1 << 2
		}
		// A known empty file and a not-yet-enriched base row both carry Size=0.
		// Keep that distinction in MetadataRevision so the later zero-byte
		// enrichment invalidates an in-flight provisional metadata snapshot.
		if entry.SizeKnown || entry.Size != 0 {
			metadataFlags |= 1 << 3
		}
		_, _ = metadata.Write([]byte{metadataFlags})
		if entry.Selected {
			selectedEntryIDs = append(selectedEntryIDs, entryID)
		}
	}
	// SelectionRevision represents the selected identity set, independently
	// of catalog ordering. Sorting already advances CatalogRevision; advancing
	// SelectionRevision as well rejects otherwise valid incremental selection
	// intents and creates needless state churn in native Gallery clients.
	sort.Strings(selectedEntryIDs)
	for _, entryID := range selectedEntryIDs {
		writeSemanticFingerprintString(selection, entryID)
	}
	return catalog.Sum64(), metadata.Sum64(), selection.Sum64()
}

func (fp *FileSystemPanel) semanticFingerprints() (uint64, uint64, uint64) {
	return fp.semanticFingerprintsForEntries(fp.entries)
}

func (fp *FileSystemPanel) updateSemanticRevisions() {
	if extUiPanelCatalogRowsIsEnabled() {
		sourceKind, _ := fp.semanticSourceInfo()
		fp.ensureSemanticPagedRevisions(sourceKind)
		return
	}
	catalog, metadata, selection := fp.semanticFingerprints()
	fp.updateSemanticRevisionsFromFingerprints(catalog, metadata, selection)
}

func (fp *FileSystemPanel) updateSemanticRevisionsFromFingerprints(
	catalog, metadata, selection uint64,
) {
	catalogChanged := !fp.semanticCatalogInitialized || catalog != fp.semanticCatalogFingerprint
	metadataChanged := !fp.semanticMetadataInitialized || metadata != fp.semanticMetadataFingerprint
	// Legacy protocol-v2 clients receive the complete entry metadata in the
	// panel catalog and have no independent metadata revision. Preserve their
	// original revision contract by advancing CatalogRevision for every
	// serialized metadata change. Negotiated clients keep the two revision
	// domains separate so metadata never invalidates the fast base catalog.
	if catalogChanged || (!extUiPanelCatalogMetadataIsEnabled() && metadataChanged) {
		fp.semanticCatalogFingerprint = catalog
		fp.semanticCatalogInitialized = true
		fp.catalogRevision++
	}
	if metadataChanged {
		fp.semanticMetadataFingerprint = metadata
		fp.semanticMetadataInitialized = true
		fp.metadataRevision++
	}
	if fp.semanticSelectionNeedsSync {
		// SetItemSelected already advanced SelectionRevision and recorded the
		// exact changed rows. Synchronize the validation fingerprint without
		// advancing the revision a second time during a later full fallback.
		fp.semanticSelectionFingerprint = selection
		fp.semanticSelectionInitialized = true
		fp.semanticSelectionNeedsSync = false
	} else if !fp.semanticSelectionInitialized || selection != fp.semanticSelectionFingerprint {
		if fp.semanticSelectionInitialized {
			// A caller changed selection without SetItemSelected. We know the new
			// aggregate state but not the exact rows, so force the rare bounded-ID
			// replacement instead of emitting an empty or incomplete sparse delta.
			fp.semanticSelectionBaseRevision = fp.selectionRevision
			fp.semanticSelectionChanges = nil
			fp.semanticSelectionOverflow = true
		}
		fp.semanticSelectionFingerprint = selection
		fp.semanticSelectionInitialized = true
		fp.selectionRevision++
	}
}

const maxSemanticSelectionChanges = 4096

type semanticSelectionChange struct {
	Index    int
	EntryID  string
	Selected bool
}

func (fp *FileSystemPanel) semanticSelectionPatch(baseRevision int64) (extui.PanelPatch, bool) {
	if fp == nil || baseRevision == fp.selectionRevision {
		return extui.PanelPatch{}, false
	}
	patch := extui.PanelPatch{
		Side:              -1,
		PanelID:           vtui.SemanticID(fp),
		CatalogRevision:   fp.catalogRevision,
		BaseSelection:     baseRevision,
		SelectionRevision: fp.selectionRevision,
	}
	if fp.semanticSelectionOverflow || baseRevision != fp.semanticSelectionBaseRevision {
		patch.Op = "selection_replace"
		patch.SelectedEntryIDs = fp.semanticSelectedEntryIDs()
		return patch, true
	}
	patch.Op = "selection_delta"
	changes := make([]semanticSelectionChange, 0, len(fp.semanticSelectionChanges))
	for _, change := range fp.semanticSelectionChanges {
		changes = append(changes, change)
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Index == changes[j].Index {
			return changes[i].EntryID < changes[j].EntryID
		}
		return changes[i].Index < changes[j].Index
	})
	patch.SelectionChanges = make([]extui.M, 0, len(changes))
	for _, change := range changes {
		patch.SelectionChanges = append(patch.SelectionChanges, extui.M{
			"index": change.Index, "entryId": change.EntryID,
			"selected": change.Selected,
		})
	}
	return patch, true
}

func (fp *FileSystemPanel) semanticSelectedEntryIDs() []string {
	if fp == nil {
		return nil
	}
	cache := fp.semanticStaticCache
	selected := make([]string, 0, len(fp.selectedItems))
	if cache != nil && cache.catalogRevision == fp.catalogRevision && len(cache.entries) == len(fp.entries) {
		for index, entry := range fp.entries {
			if entry.Selected {
				selected = append(selected, cache.entries[index].EntryID)
			}
		}
	} else {
		sourceKind, _ := fp.semanticSourceInfo()
		for _, entry := range fp.entries {
			if !entry.Selected {
				continue
			}
			entryID, _ := fp.semanticEntryMetadata(entry, sourceKind)
			selected = append(selected, entryID)
		}
	}
	sort.Strings(selected)
	return selected
}

func (fp *FileSystemPanel) acknowledgeSemanticSelection(revision int64) {
	if fp == nil || revision != fp.selectionRevision {
		return
	}
	fp.semanticSelectionBaseRevision = revision
	fp.semanticSelectionChanges = nil
	fp.semanticSelectionOverflow = false
}

type semanticPanelStaticCache struct {
	catalogRevision       int64
	separateFileExtension bool
	highlighterRevision   int64
	highlightStyles       map[string]extui.HighlightStyleModel
	entries               []extui.FileEntryModel
	entryIndexByID        map[string]int
	mediaBroker           *extUiMediaBroker
	mediaSourceEpoch      int64
}

func emptySemanticSelectionFingerprint() uint64 {
	return fnv.New64a().Sum64()
}

func (fp *FileSystemPanel) semanticStaticPanelData(sourceKind string) *semanticPanelStaticCache {
	mediaBroker := currentExtUiMediaBroker()
	metadataDeferred := extUiPanelCatalogMetadataIsEnabled()
	benchmark := navigationBenchmarkCurrentUI()
	cache := fp.semanticStaticCache
	if cache != nil &&
		cache.catalogRevision == fp.catalogRevision &&
		cache.separateFileExtension == AppConfig.SeparateFileExtensions &&
		cache.highlighterRevision == semanticHighlighterRevision() &&
		cache.mediaBroker == mediaBroker &&
		cache.mediaSourceEpoch == fp.mediaSourceEpoch {
		return cache
	}

	imageExtensions := semanticImageExtensions()
	cache = &semanticPanelStaticCache{
		catalogRevision:       fp.catalogRevision,
		separateFileExtension: AppConfig.SeparateFileExtensions,
		highlighterRevision:   semanticHighlighterRevision(),
		highlightStyles:       make(map[string]extui.HighlightStyleModel),
		entries:               make([]extui.FileEntryModel, 0, len(fp.entries)),
		entryIndexByID:        make(map[string]int, len(fp.entries)),
		mediaBroker:           mediaBroker,
		mediaSourceEpoch:      fp.mediaSourceEpoch,
	}
	panelID := vtui.SemanticID(fp)
	resourceIDs := make([]string, 0, len(fp.entries))
	caps := fp.vfs.GetCapabilities()
	rowsStartedNs := int64(0)
	if benchmark != nil {
		rowsStartedNs = navigationBenchmarkMonotonicNs()
	}
	imageCount := 0
	styleDurationNs := int64(0)
	sourceDurationNs := int64(0)
	// On a cold large directory the base scene is meant to expose names and
	// types immediately. Even the cheap/provisional highlighter walk is still
	// O(N), and its result is superseded by the bounded metadata stream anyway.
	// Keep the eager path for small catalogs (and for complete catalogs) where
	// the first-frame appearance is worth the tiny traversal.
	const eagerDeferredStyleRows = 256
	eagerStyle := !metadataDeferred || len(fp.entries) <= eagerDeferredStyleRows
	for i, entry := range fp.entries {
		entryID, logicalPath := fp.semanticEntryMetadata(entry, sourceKind)
		cache.entryIndexByID[entryID] = i
		isImage := semanticEntryIsImage(entry, imageExtensions)
		if isImage {
			imageCount++
		}
		displayBaseName := entry.Name
		displayExtension := ""
		if AppConfig.SeparateFileExtensions && !entry.NoExtension && !entry.IsDir && entry.Name != ".." {
			if base, extension := splitFileExtension(entry.Name); extension != "" {
				displayBaseName = base
				displayExtension = extension
			}
		}
		// Only name/dir/hidden-based rules can resolve here: entry.VFSItem
		// carries just the fast base-pass fields at this point, and
		// SemanticStyle(..., false) skips any rule that needs more (see
		// HighlightRule.hasDeferredPredicate). The metadata pass
		// (semanticPanelModel's !metadataDeferred loop and
		// BuildPanelCatalogMetadataChunk) recomputes with the complete item
		// and overwrites this provisional style.
		highlightStyleID := ""
		var highlightStyle extui.HighlightStyleModel
		if eagerStyle {
			styleStartedNs := int64(0)
			if benchmark != nil {
				styleStartedNs = navigationBenchmarkMonotonicNs()
			}
			highlightStyleID, highlightStyle = GlobalFileHighlighter.SemanticStyle(
				&entry.VFSItem, false)
			if benchmark != nil {
				styleDurationNs += navigationBenchmarkMonotonicNs() - styleStartedNs
			}
		}
		if highlightStyleID != "" {
			cache.highlightStyles[highlightStyleID] = highlightStyle
		}
		// In the negotiated deferred-metadata protocol, the base catalog only
		// needs a broker source for rows which can actually request a thumbnail.
		// Registering every non-directory (including thousands of DLL/TXT rows)
		// creates a broker resource, hashes several identities, and takes the
		// broker mutex once per row while Enter is waiting for the first catalog.
		// Non-image rows get their path/metadata in the bounded metadata stream and
		// never enter ZoinGallery's image pipeline, so omitting their descriptor is
		// both safe and materially cheaper. Legacy complete catalogs retain the
		// previous descriptor contract.
		needsSourceDescriptor := !metadataDeferred || isImage
		version := ""
		var sourceModel *extui.ImageSourceModel
		sourceStartedNs := int64(0)
		if benchmark != nil {
			sourceStartedNs = navigationBenchmarkMonotonicNs()
		}
		if needsSourceDescriptor {
			entryPath := logicalPath
			storage := mediaStorageClass(caps, "")
			var versionStrength string
			version, versionStrength = mediaSourceVersion(fp.vfs, entry.VFSItem, storage == vfs.StorageClassLocal, fp.catalogRevision, fp.mediaSourceEpoch)
			source := extui.ImageSourceModel{
				SourceKey:       mediaSourceKey(fp.vfs, entryPath),
				Version:         version,
				VersionStrength: versionStrength,
				Size:            entry.Size,
				SizeKnown:       entry.SizeKnown || entry.Size != 0,
				AccessProfile:   caps.ReadAccess.String(),
				StorageClass:    storage.String(),
			}
			if mediaBroker != nil && !entry.IsDir && entry.Name != ".." {
				descriptor := mediaBroker.Register(mediaSourceRegistration{
					PanelID: panelID, CatalogVersion: fp.catalogRevision, SourceEpoch: fp.mediaSourceEpoch, FS: fp.vfs,
					Path: entryPath, Item: entry.VFSItem,
				})
				source = extui.ImageSourceModel{
					ResourceID: descriptor.ResourceID, SourceKey: descriptor.SourceKey,
					Version: descriptor.Version, VersionStrength: descriptor.VersionStrength,
					Size: descriptor.Size, SizeKnown: descriptor.SizeKnown,
					AccessProfile: descriptor.AccessProfile, StorageClass: descriptor.StorageClass,
				}
				resourceIDs = append(resourceIDs, descriptor.ResourceID)
			}
			if !entry.IsDir && entry.Name != ".." {
				sourceModel = &source
			}
		}
		if benchmark != nil {
			sourceDurationNs += navigationBenchmarkMonotonicNs() - sourceStartedNs
		}
		cache.entries = append(cache.entries, extui.FileEntryModel{
			Index:            i,
			EntryID:          entryID,
			Name:             entry.Name,
			DisplayBaseName:  displayBaseName,
			DisplayExtension: displayExtension,
			Path:             logicalPath,
			IsDir:            entry.IsDir,
			IsUp:             entry.Name == "..",
			IsHidden:         entry.IsHidden,
			IsImage:          isImage,
			Version:          version,
			Source:           sourceModel,
			HighlightStyleID: highlightStyleID,
		})
	}
	if benchmark != nil {
		rowsFinishedNs := navigationBenchmarkMonotonicNs()
		benchmark.eventAt("model.semantic_static.rows", "go.ui", rowsFinishedNs,
			"entries", len(fp.entries), "images", imageCount,
			"styleDurationNs", styleDurationNs, "sourceDurationNs", sourceDurationNs,
			"durationNs", rowsFinishedNs-rowsStartedNs)
	}
	if mediaBroker != nil {
		commitStartedNs := int64(0)
		if benchmark != nil {
			commitStartedNs = navigationBenchmarkMonotonicNs()
		}
		mediaBroker.CommitPanel(panelID, fp.catalogRevision, resourceIDs)
		if benchmark != nil {
			commitFinishedNs := navigationBenchmarkMonotonicNs()
			benchmark.eventAt("model.semantic_static.commit", "go.ui", commitFinishedNs,
				"resources", len(resourceIDs),
				"durationNs", commitFinishedNs-commitStartedNs)
		}
	}
	fp.semanticStaticCache = cache
	return cache
}

type semanticPanelMetadataEntry struct {
	index          int
	entryID        string
	logicalPath    string
	item           vfs.VFSItem
	sizeCalculated bool
}

type semanticPanelMetadataSnapshot struct {
	panelID           string
	path              string
	catalogRevision   int64
	metadataRevision  int64
	highlightRevision int64
	provider          vfs.LocalPathProvider
	highlighter       *FileHighlighter
	entries           []semanticPanelMetadataEntry
	totalSize         int64
}

var semanticPanelMetadataSnapshots sync.Map // panel semantic ID -> *semanticPanelMetadataSnapshot

// unpublishSemanticMetadataSnapshot releases the process-global reference to
// a panel catalog when its owning workspace is disposed. CompareAndDelete is
// intentional: a stale panel must never remove a newer snapshot which happens
// to have been published under the same semantic ID.
func (fp *FileSystemPanel) unpublishSemanticMetadataSnapshot() {
	if fp == nil {
		return
	}
	semanticLivePanels.CompareAndDelete(vtui.SemanticID(fp), fp)
	if fp.semanticMetadataSnapshot == nil {
		return
	}
	snapshot := fp.semanticMetadataSnapshot
	fp.semanticMetadataSnapshot = nil
	semanticPanelMetadataSnapshots.CompareAndDelete(snapshot.panelID, snapshot)
}

func cloneSemanticFileHighlighter(source *FileHighlighter) *FileHighlighter {
	if source == nil {
		return nil
	}
	clone := &FileHighlighter{Revision: source.Revision, Rules: make([]HighlightRule, len(source.Rules))}
	copy(clone.Rules, source.Rules)
	for i := range clone.Rules {
		clone.Rules[i].Masks = append([]string(nil), source.Rules[i].Masks...)
	}
	return clone
}

func (fp *FileSystemPanel) publishSemanticMetadataSnapshot(panelID string, baseEntries []extui.FileEntryModel) {
	path := fp.vfs.GetPath()
	cache := fp.semanticMetadataSnapshot
	if cache != nil && cache.panelID == panelID && cache.path == path &&
		cache.catalogRevision == fp.catalogRevision && cache.metadataRevision == fp.metadataRevision {
		semanticPanelMetadataSnapshots.Store(panelID, cache)
		return
	}

	snapshot := &semanticPanelMetadataSnapshot{
		panelID:           panelID,
		path:              path,
		catalogRevision:   fp.catalogRevision,
		metadataRevision:  fp.metadataRevision,
		highlightRevision: semanticHighlighterRevision(),
		highlighter:       cloneSemanticFileHighlighter(GlobalFileHighlighter),
		entries:           make([]semanticPanelMetadataEntry, 0, len(fp.entries)),
	}
	if provider, ok := fp.vfs.(vfs.LocalPathProvider); ok {
		snapshot.provider = provider
	}
	for index, entry := range fp.entries {
		base := baseEntries[index]
		snapshot.entries = append(snapshot.entries, semanticPanelMetadataEntry{
			index:          index,
			entryID:        base.EntryID,
			logicalPath:    base.Path,
			item:           entry.VFSItem,
			sizeCalculated: entry.SizeCalculated,
		})
		if !entry.IsDir {
			snapshot.totalSize += entry.Size
		}
	}
	fp.semanticMetadataSnapshot = snapshot
	semanticPanelMetadataSnapshots.Store(panelID, snapshot)
}

// commitSemanticMetadataMutation advances the deferred metadata revision and
// replaces the pull snapshot at the exact point where panel rows were
// enriched. Keep this out of semanticPanelHeaderModel: that exporter runs on
// ordinary cursor/layout redraws and must remain row-free. Actual metadata
// commits are rare and already touch every changed row, so one fingerprint
// pass here preserves the compact state_update path without adding per-frame
// catalog work.
func (fp *FileSystemPanel) commitSemanticMetadataMutation() {
	if fp == nil || !extUiPanelCatalogMetadataIsEnabled() {
		return
	}
	if extUiPanelCatalogRowsIsEnabled() {
		sourceKind, _ := fp.semanticSourceInfo()
		fp.ensureSemanticPagedRevisions(sourceKind)
		fp.metadataRevision++
		return
	}
	fp.updateSemanticRevisions()
	static := fp.semanticStaticCache
	if static == nil || static.catalogRevision != fp.catalogRevision ||
		len(static.entries) != len(fp.entries) {
		// A concurrent identity/order change requires the normal full-catalog
		// fallback. semanticPanelHeaderModel will reject this stale cache.
		return
	}
	fp.publishSemanticMetadataSnapshot(vtui.SemanticID(fp), static.entries)
}

const (
	defaultPanelCatalogMetadataChunkLimit = 8
	maxPanelCatalogMetadataChunkLimit     = 128
)

// BuildPanelCatalogMetadataChunk returns exactly one bounded deferred metadata
// chunk. Every request is checked against the latest published base snapshot;
// a navigation or metadata change therefore makes an old request fail cleanly.
func BuildPanelCatalogMetadataChunk(panelID, path string, catalogRevision, metadataRevision int64,
	offset, limit int) (map[string]any, bool) {
	loaded, ok := semanticPanelMetadataSnapshots.Load(panelID)
	if !ok {
		return nil, false
	}
	snapshot, ok := loaded.(*semanticPanelMetadataSnapshot)
	if !ok || snapshot == nil || snapshot.panelID != panelID || snapshot.path != path ||
		snapshot.catalogRevision != catalogRevision || snapshot.metadataRevision != metadataRevision {
		return nil, false
	}
	if offset < 0 || offset > len(snapshot.entries) {
		return nil, false
	}
	if limit <= 0 {
		limit = defaultPanelCatalogMetadataChunkLimit
	}
	if limit > maxPanelCatalogMetadataChunkLimit {
		limit = maxPanelCatalogMetadataChunkLimit
	}
	end := offset + limit
	if end > len(snapshot.entries) {
		end = len(snapshot.entries)
	}

	chunk := extui.PanelCatalogMetadataModel{
		PanelID:           panelID,
		Path:              path,
		CatalogRevision:   catalogRevision,
		MetadataRevision:  metadataRevision,
		HighlightRevision: snapshot.highlightRevision,
		Offset:            offset,
		Limit:             limit,
		Total:             len(snapshot.entries),
		TotalSize:         snapshot.totalSize,
		Final:             end == len(snapshot.entries),
		Entries:           make([]extui.FileEntryMetadataModel, 0, end-offset),
		HighlightStyles:   make(map[string]extui.HighlightStyleModel),
	}
	for _, source := range snapshot.entries[offset:end] {
		localPath := ""
		if snapshot.provider != nil {
			if resolved, err := snapshot.provider.LocalPath(source.logicalPath); err == nil {
				localPath = resolved
			}
		}
		highlightStyleID, highlightStyle := snapshot.highlighter.SemanticStyle(&source.item, true)
		if highlightStyleID != "" {
			chunk.HighlightStyles[highlightStyleID] = highlightStyle
		}
		entry := fileEntry{VFSItem: source.item, SizeCalculated: source.sizeCalculated}
		mtimeNanos := semanticMTimeNanos(source.item.MTime)
		chunk.Entries = append(chunk.Entries, extui.FileEntryMetadataModel{
			Index:            source.index,
			EntryID:          source.entryID,
			LocalPath:        localPath,
			Size:             semanticFileSizeValue(&entry),
			SizeText:         semanticFileSize(&entry),
			MTime:            source.item.MTime.Format("2006-01-02 15:04"),
			MTimeNanos:       mtimeNanos,
			Mode:             source.item.Mode,
			HighlightStyleID: highlightStyleID,
		})
	}
	return chunk.ToMap(), true
}

const (
	// A details viewport paints about 39 rows at the default density. Forty-
	// eight leaves a small scroll overscan while keeping cold catalog resets
	// bounded to rows which can affect the first frame.
	initialPanelCatalogRowsLimit = 48
	maxPanelCatalogRowsLimit     = 256
	// Fast Find is transient viewport decoration. Keep its exported match map
	// bounded to painted/buffered rows instead of serializing one entry for an
	// entire large directory on every keystroke.
	semanticFastFindRowsLimit        = maxPanelCatalogRowsLimit
	semanticFastFindDetailsRowsLimit = 128
)

// semanticLivePanels is an ownership registry, not a row cache. Requests are
// executed on the UI thread and read the current FileSystemPanel directly;
// no converted catalog survives a navigation and no second 30k-row copy is
// retained merely for the native frontend.
var semanticLivePanels sync.Map // panel semantic ID -> *FileSystemPanel

func (fp *FileSystemPanel) markSemanticCatalogMutation() {
	if fp == nil {
		return
	}
	fp.semanticCatalogGeneration++
	fp.semanticStaticCache = nil
}

func (fp *FileSystemPanel) semanticPagedModelSignature(sourceKind string) string {
	if fp == nil || fp.vfs == nil {
		return ""
	}
	return fmt.Sprintf("%s\x00%T\x00%s\x00%d\x00%t\x00%t\x00%d",
		sourceKind, fp.vfs, fp.vfs.GetPath(), fp.sortMode, fp.sortReverse,
		AppConfig.SeparateFileExtensions, fp.mediaSourceEpoch)
}

// ensureSemanticPagedRevisions advances revision domains from known panel
// mutations. The ordinary compatibility exporter still fingerprints every
// row, but a negotiated paged client must not spend O(N) before it can paint
// an O(viewport) model.
func (fp *FileSystemPanel) ensureSemanticPagedRevisions(sourceKind string) {
	if fp == nil {
		return
	}
	signature := fp.semanticPagedModelSignature(sourceKind)
	if !fp.semanticCatalogInitialized ||
		fp.semanticPublishedGeneration != fp.semanticCatalogGeneration ||
		fp.semanticPagedSignature != signature {
		fp.catalogRevision++
		fp.metadataRevision++
		fp.semanticCatalogInitialized = true
		fp.semanticMetadataInitialized = true
		fp.semanticPublishedGeneration = fp.semanticCatalogGeneration
		fp.semanticPagedSignature = signature
		fp.semanticStaticCache = nil
		fp.unpublishSemanticMetadataSnapshot()
		fp.semanticPagedResourceRevision = 0
		fp.semanticPagedResourceIDs = nil
	}
	if !fp.semanticSelectionInitialized {
		fp.semanticSelectionInitialized = true
		fp.semanticSelectionFingerprint = emptySemanticSelectionFingerprint()
		fp.selectionRevision++
	}
}

func semanticPanelCatalogRange(total, cursor, limit int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if limit <= 0 || limit > total {
		limit = total
	}
	if cursor < 0 || cursor >= total {
		cursor = 0
	}
	offset := cursor - limit/2
	if offset < 0 {
		offset = 0
	}
	if maximum := total - limit; offset > maximum {
		offset = maximum
	}
	return offset, offset + limit
}

func (fp *FileSystemPanel) semanticFastFindRange() (int, int, bool) {
	if fp == nil || !fp.fastFindMode || fp.fastFindStr == "" || len(fp.entries) == 0 {
		return 0, 0, false
	}
	first, end := semanticFastFindWindowRange(
		len(fp.entries), fp.GetCursorIndex(), fp.semanticFastFindWindowLimit())
	return first, end, true
}

func (fp *FileSystemPanel) semanticFastFindWindowLimit() int {
	// Details paints about 39 rows and keeps one viewport of delegates on
	// either side. A stable 128-row window therefore covers the complete
	// visible/overscan set while halving the map sent for every query edit.
	// Tile/column layouts can expose substantially more entries at once and
	// retain the wider general-purpose window.
	if fp != nil && fp.effectiveGalleryLayoutMode() == GalleryLayoutDetails {
		return semanticFastFindDetailsRowsLimit
	}
	return semanticFastFindRowsLimit
}

// semanticFastFindWindowRange keeps the bounded highlight window stable while
// the cursor moves inside it. Re-centering on every Ctrl+Enter changed two map
// keys but forced the entire map through the semantic patch and QML binding on
// every step. A quarter-window stride leaves at least three eighths of the
// window on either side of the cursor and replaces it only once per stride.
func semanticFastFindWindowRange(total, cursor, limit int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if limit <= 0 || limit >= total {
		return 0, total
	}
	cursor = max(0, min(cursor, total-1))
	stride := max(1, limit/4)
	leadingMargin := (limit - stride) / 2
	offset := cursor/stride*stride - leadingMargin
	offset = max(0, min(offset, total-limit))
	return offset, offset + limit
}

func (fp *FileSystemPanel) semanticFastFindMatches(
	first, end int, entryIDAt func(int) string,
) map[string]extui.FastFindMatchModel {
	if fp == nil || !fp.fastFindMode || fp.fastFindStr == "" || entryIDAt == nil {
		return nil
	}
	fp.ensureFastFindMatchCache()
	if fp.fastFindAnyMatchKnown && !fp.fastFindAnyMatch {
		return nil
	}
	first = max(0, first)
	end = min(len(fp.entries), end)
	if first >= end {
		return nil
	}
	matches := make(map[string]extui.FastFindMatchModel, end-first)
	for index := first; index < end; index++ {
		start, length, ok := fp.fastFindMatchAt(index)
		if !ok || length <= 0 {
			continue
		}
		if entryID := entryIDAt(index); entryID != "" {
			matches[entryID] = extui.FastFindMatchModel{
				Start: start, Length: length,
			}
		}
	}
	return matches
}

func (fp *FileSystemPanel) semanticPagedRows(offset, limit int) (
	[]extui.FileEntryModel, map[string]extui.HighlightStyleModel, bool,
) {
	if fp == nil || fp.vfs == nil || offset < 0 || offset > len(fp.entries) {
		return nil, nil, false
	}
	if limit <= 0 {
		limit = initialPanelCatalogRowsLimit
	}
	if limit > maxPanelCatalogRowsLimit {
		limit = maxPanelCatalogRowsLimit
	}
	end := offset + limit
	if end > len(fp.entries) {
		end = len(fp.entries)
	}
	sourceKind, _ := fp.semanticSourceInfo()
	imageExtensions := semanticImageExtensions()
	styles := make(map[string]extui.HighlightStyleModel)
	rows := make([]extui.FileEntryModel, 0, end-offset)
	mediaBroker := currentExtUiMediaBroker()
	panelID := vtui.SemanticID(fp)
	if fp.semanticPagedResourceRevision != fp.catalogRevision {
		fp.semanticPagedResourceRevision = fp.catalogRevision
		fp.semanticPagedResourceIDs = make(map[string]struct{})
	}
	caps := fp.vfs.GetCapabilities()
	for index := offset; index < end; index++ {
		entry := fp.entries[index]
		if entry == nil {
			return nil, nil, false
		}
		entryID, logicalPath := fp.semanticEntryMetadata(entry, sourceKind)
		displayBaseName := entry.Name
		displayExtension := ""
		if AppConfig.SeparateFileExtensions && !entry.NoExtension &&
			!entry.IsDir && entry.Name != ".." {
			if base, extension := splitFileExtension(entry.Name); extension != "" {
				displayBaseName, displayExtension = base, extension
			}
		}
		highlightStyleID, highlightStyle := GlobalFileHighlighter.SemanticStyle(
			&entry.VFSItem, false)
		if highlightStyleID != "" {
			styles[highlightStyleID] = highlightStyle
		}
		isImage := semanticEntryIsImage(entry, imageExtensions)
		version := ""
		var sourceModel *extui.ImageSourceModel
		if isImage {
			storage := mediaStorageClass(caps, "")
			var versionStrength string
			version, versionStrength = mediaSourceVersion(
				fp.vfs, entry.VFSItem, storage == vfs.StorageClassLocal,
				fp.catalogRevision, fp.mediaSourceEpoch)
			source := extui.ImageSourceModel{
				SourceKey:       mediaSourceKey(fp.vfs, logicalPath),
				Version:         version,
				VersionStrength: versionStrength,
				Size:            entry.Size,
				SizeKnown:       entry.SizeKnown || entry.Size != 0,
				AccessProfile:   caps.ReadAccess.String(),
				StorageClass:    storage.String(),
			}
			if mediaBroker != nil {
				descriptor := mediaBroker.Register(mediaSourceRegistration{
					PanelID: panelID, CatalogVersion: fp.catalogRevision,
					SourceEpoch: fp.mediaSourceEpoch, FS: fp.vfs,
					Path: logicalPath, Item: entry.VFSItem,
				})
				source = extui.ImageSourceModel{
					ResourceID: descriptor.ResourceID, SourceKey: descriptor.SourceKey,
					Version: descriptor.Version, VersionStrength: descriptor.VersionStrength,
					Size: descriptor.Size, SizeKnown: descriptor.SizeKnown,
					AccessProfile: descriptor.AccessProfile, StorageClass: descriptor.StorageClass,
				}
				if descriptor.ResourceID != "" {
					fp.semanticPagedResourceIDs[descriptor.ResourceID] = struct{}{}
				}
			}
			sourceModel = &source
		}
		rows = append(rows, extui.FileEntryModel{
			Index: index, EntryID: entryID, Name: entry.Name,
			DisplayBaseName: displayBaseName, DisplayExtension: displayExtension,
			Path: logicalPath, IsDir: entry.IsDir, IsUp: entry.Name == "..",
			IsHidden: entry.IsHidden, IsImage: isImage, Selected: entry.Selected,
			Version: version, Source: sourceModel, HighlightStyleID: highlightStyleID,
		})
	}
	if mediaBroker != nil {
		resourceIDs := make([]string, 0, len(fp.semanticPagedResourceIDs))
		for resourceID := range fp.semanticPagedResourceIDs {
			resourceIDs = append(resourceIDs, resourceID)
		}
		mediaBroker.CommitPanel(panelID, fp.catalogRevision, resourceIDs)
	}
	return rows, styles, true
}

func BuildLivePanelCatalogRows(panelID, path string, catalogRevision int64,
	offset, limit int,
) (map[string]any, bool) {
	loaded, ok := semanticLivePanels.Load(panelID)
	if !ok {
		return nil, false
	}
	fp, ok := loaded.(*FileSystemPanel)
	if !ok || fp == nil || fp.vfs == nil || fp.vfs.GetPath() != path ||
		fp.catalogRevision != catalogRevision || offset < 0 || offset > len(fp.entries) {
		return nil, false
	}
	rows, styles, ok := fp.semanticPagedRows(offset, limit)
	if !ok || len(rows) == 0 && offset != len(fp.entries) {
		return nil, false
	}
	return extui.PanelCatalogRowsModel{
		PanelID: panelID, Path: path, CatalogRevision: catalogRevision,
		Offset: offset, Limit: len(rows), Total: len(fp.entries),
		Entries: rows, HighlightStyles: styles,
	}.ToMap(), true
}

func BuildLivePanelCatalogMetadataChunk(panelID, path string,
	catalogRevision, metadataRevision int64, offset, limit int,
) (map[string]any, bool) {
	loaded, ok := semanticLivePanels.Load(panelID)
	if !ok {
		return nil, false
	}
	fp, ok := loaded.(*FileSystemPanel)
	if !ok || fp == nil || fp.vfs == nil || fp.vfs.GetPath() != path ||
		fp.catalogRevision != catalogRevision || fp.metadataRevision != metadataRevision ||
		offset < 0 || offset > len(fp.entries) {
		return nil, false
	}
	if limit <= 0 {
		limit = defaultPanelCatalogMetadataChunkLimit
	}
	if limit > maxPanelCatalogMetadataChunkLimit {
		limit = maxPanelCatalogMetadataChunkLimit
	}
	end := offset + limit
	if end > len(fp.entries) {
		end = len(fp.entries)
	}
	sourceKind, _ := fp.semanticSourceInfo()
	provider, _ := fp.vfs.(vfs.LocalPathProvider)
	chunk := extui.PanelCatalogMetadataModel{
		PanelID: panelID, Path: path, CatalogRevision: catalogRevision,
		MetadataRevision:  metadataRevision,
		HighlightRevision: semanticHighlighterRevision(),
		Offset:            offset, Limit: end - offset, Total: len(fp.entries),
		Final:           end == len(fp.entries),
		Entries:         make([]extui.FileEntryMetadataModel, 0, end-offset),
		HighlightStyles: make(map[string]extui.HighlightStyleModel),
	}
	for index := offset; index < end; index++ {
		entry := fp.entries[index]
		if entry == nil {
			return nil, false
		}
		entryID, logicalPath := fp.semanticEntryMetadata(entry, sourceKind)
		localPath := ""
		if provider != nil {
			if resolved, err := provider.LocalPath(logicalPath); err == nil {
				localPath = resolved
			}
		}
		highlightStyleID, highlightStyle := GlobalFileHighlighter.SemanticStyle(
			&entry.VFSItem, true)
		if highlightStyleID != "" {
			chunk.HighlightStyles[highlightStyleID] = highlightStyle
		}
		if !entry.IsDir {
			chunk.TotalSize += entry.Size
		}
		chunk.Entries = append(chunk.Entries, extui.FileEntryMetadataModel{
			Index: index, EntryID: entryID, LocalPath: localPath,
			Size: semanticFileSizeValue(entry), SizeText: semanticFileSize(entry),
			MTime:      entry.MTime.Format("2006-01-02 15:04"),
			MTimeNanos: semanticMTimeNanos(entry.MTime), Mode: entry.Mode,
			HighlightStyleID: highlightStyleID,
		})
	}
	return chunk.ToMap(), true
}

func (fp *FileSystemPanel) semanticPagedPanelModel(
	ctx *vtui.SemanticContext, side int, active bool,
) extui.PanelModel {
	sourceKind, previewCapable := fp.semanticSourceInfo()
	fp.ensureSemanticPagedRevisions(sourceKind)
	panelID := vtui.SemanticID(fp)
	semanticLivePanels.Store(panelID, fp)
	cursor := fp.GetCursorIndex()
	offset, end := semanticPanelCatalogRange(
		len(fp.entries), cursor, initialPanelCatalogRowsLimit)
	entries, highlightStyles, _ := fp.semanticPagedRows(offset, end-offset)
	cursorEntryID := ""
	if cursor >= 0 && cursor < len(fp.entries) {
		cursorEntryID, _ = fp.semanticEntryMetadata(fp.entries[cursor], sourceKind)
	}
	fastFindMatches := fp.semanticFastFindMatches(
		offset, end, func(index int) string {
			return entries[index-offset].EntryID
		})
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.galleryLayoutRevision
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	semanticTitle := fp.semanticTitle
	if semanticTitle == "" {
		semanticTitle = fp.currentTitle
	}
	return extui.PanelModel{
		ID: panelID, Side: side, Active: active, Path: fp.vfs.GetPath(),
		Title: semanticTitle, ShowFileInfo: AppConfig.ShowPanelFileInfo,
		GalleryLayoutMode:     string(galleryLayoutMode),
		GalleryColumnCount:    fp.effectiveGalleryColumnCount(),
		GalleryDensity:        fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:      fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision: galleryLayoutRevision,
		SourceKind:            sourceKind, PreviewCapable: previewCapable,
		CatalogRevision:   fp.catalogRevision,
		SelectionRevision: fp.selectionRevision,
		MetadataDeferred:  true, MetadataRevision: fp.metadataRevision,
		CatalogRowsDeferred: true,
		HighlightRevision:   semanticHighlighterRevision(),
		HighlightStyles:     highlightStyles,
		CursorEntryID:       cursorEntryID,
		SortMode:            sortModeName(fp.sortMode), SortReverse: fp.sortReverse,
		SeparateFileExtensions: AppConfig.SeparateFileExtensions,
		Cursor:                 cursor, Loading: fp.semanticLoading(),
		CatalogProvisional: fp.catalogProvisional,
		FastFind:           fp.fastFindMode, FastFindText: fp.fastFindStr,
		FastFindMatchColor: func() string {
			if fp.fastFindMode && fp.fastFindStr != "" {
				return semanticAttrColor(vtui.Palette[ColPanelHighlightText], true)
			}
			return ""
		}(),
		FastFindMatches: fastFindMatches,
		SelectedCount:   len(fp.selectedItems), TotalCount: len(fp.entries),
		GalleryColumns: fp.semanticGalleryColumns(), Entries: entries,
	}
}

func (fp *FileSystemPanel) semanticLoading() bool {
	return fp != nil && fp.isLoading && !fp.catalogInteractive
}

func (fp *FileSystemPanel) semanticPanelModel(ctx *vtui.SemanticContext, side int, active bool) extui.PanelModel {
	if extUiPanelCatalogRowsIsEnabled() {
		return fp.semanticPagedPanelModel(ctx, side, active)
	}
	benchmark := navigationBenchmarkCurrentUI()
	metadataDeferred := extUiPanelCatalogMetadataIsEnabled()
	sourceKind, previewCapable := fp.semanticSourceInfo()
	revisionsStartedNs := int64(0)
	if benchmark != nil {
		revisionsStartedNs = navigationBenchmarkMonotonicNs()
	}
	fp.updateSemanticRevisions()
	if benchmark != nil {
		revisionsFinishedNs := navigationBenchmarkMonotonicNs()
		benchmark.eventAt("model.semantic_revisions", "go.ui", revisionsFinishedNs,
			"durationNs", revisionsFinishedNs-revisionsStartedNs)
	}
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.galleryLayoutRevision
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	static := fp.semanticStaticPanelData(sourceKind)
	entries := static.entries
	var fastFindMatches map[string]extui.FastFindMatchModel
	fastFindMatchColor := ""
	if first, end, active := fp.semanticFastFindRange(); active {
		fastFindMatches = fp.semanticFastFindMatches(
			first, end, func(index int) string {
				return static.entries[index].EntryID
			})
		fastFindMatchColor = semanticAttrColor(
			vtui.Palette[ColPanelHighlightText], true)
	}
	selectedCount := 0
	var selectedSize int64
	var totalSize int64
	highlightRevision := int64(0)
	var highlightStyles map[string]extui.HighlightStyleModel
	var localPathProvider vfs.LocalPathProvider
	if !metadataDeferred {
		highlightRevision = semanticHighlighterRevision()
		highlightStyles = make(map[string]extui.HighlightStyleModel)
		localPathProvider, _ = fp.vfs.(vfs.LocalPathProvider)
	} else {
		// entries already carries each row's provisional (name/dir/hidden
		// rule) HighlightStyleID copied from static.entries above; expose the
		// styles it references so the frontend can resolve them immediately
		// instead of waiting for the metadata pass.
		highlightRevision = static.highlighterRevision
		highlightStyles = static.highlightStyles
	}
	needsDynamicRows := !metadataDeferred ||
		fp.semanticSelectionFingerprint != emptySemanticSelectionFingerprint()
	if needsDynamicRows {
		entries = append([]extui.FileEntryModel(nil), static.entries...)
	}
	for i, entry := range fp.entries {
		if !needsDynamicRows {
			break
		}
		entries[i].Selected = entry.Selected
		if entry.Selected {
			selectedCount++
		}
		if metadataDeferred {
			continue
		}
		if entry.Selected {
			selectedSize += entry.Size
		}
		if !entry.IsDir {
			totalSize += entry.Size
		}

		if localPathProvider != nil {
			if resolved, err := localPathProvider.LocalPath(entries[i].Path); err == nil {
				entries[i].LocalPath = resolved
			}
		}
		entries[i].Size = semanticFileSizeValue(entry)
		entries[i].SizeText = semanticFileSize(entry)
		entries[i].IsHidden = entry.IsHidden
		entries[i].IsExecutable = entry.IsExecutable
		entries[i].SizeCalculated = entry.SizeCalculated
		entries[i].MTime = entry.MTime.Format("2006-01-02 15:04")
		entries[i].MTimeNanos = semanticMTimeNanos(entry.MTime)
		entries[i].Version = fmt.Sprintf("%d:%d", entries[i].MTimeNanos, entry.Size)
		entries[i].Mode = entry.Mode
		highlightStyleID, highlightStyle := GlobalFileHighlighter.SemanticStyle(&entry.VFSItem, true)
		entries[i].HighlightStyleID = highlightStyleID
		if highlightStyleID != "" {
			highlightStyles[highlightStyleID] = highlightStyle
		}
	}
	cursorEntryID := ""
	if cursor := fp.GetCursorIndex(); cursor >= 0 && cursor < len(entries) {
		cursorEntryID = entries[cursor].EntryID
	}
	semanticTitle := fp.semanticTitle
	if semanticTitle == "" {
		// Compatibility for restored/test panels constructed without calling
		// updateTitle. Production panels always keep this free of TUI chrome.
		semanticTitle = fp.currentTitle
	}
	panelID := vtui.SemanticID(fp)
	if metadataDeferred {
		fp.publishSemanticMetadataSnapshot(panelID, static.entries)
	} else {
		fp.unpublishSemanticMetadataSnapshot()
	}
	return extui.PanelModel{
		ID:                     panelID,
		Side:                   side,
		Active:                 active,
		Path:                   fp.vfs.GetPath(),
		Title:                  semanticTitle,
		ShowFileInfo:           AppConfig.ShowPanelFileInfo,
		GalleryLayoutMode:      string(galleryLayoutMode),
		GalleryColumnCount:     fp.effectiveGalleryColumnCount(),
		GalleryDensity:         fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:       fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision:  galleryLayoutRevision,
		SourceKind:             sourceKind,
		PreviewCapable:         previewCapable,
		CatalogRevision:        fp.catalogRevision,
		SelectionRevision:      fp.selectionRevision,
		MetadataDeferred:       metadataDeferred,
		MetadataRevision:       fp.metadataRevision,
		HighlightRevision:      highlightRevision,
		HighlightStyles:        highlightStyles,
		CursorEntryID:          cursorEntryID,
		SortMode:               sortModeName(fp.sortMode),
		SortReverse:            fp.sortReverse,
		SeparateFileExtensions: AppConfig.SeparateFileExtensions,
		Cursor:                 fp.GetCursorIndex(),
		Loading:                fp.semanticLoading(),
		CatalogProvisional:     fp.catalogProvisional,
		FastFind:               fp.fastFindMode,
		FastFindText:           fp.fastFindStr,
		FastFindMatchColor:     fastFindMatchColor,
		FastFindMatches:        fastFindMatches,
		SelectedCount:          selectedCount,
		SelectedSize:           selectedSize,
		TotalCount:             len(fp.entries),
		TotalSize:              totalSize,
		GalleryColumns:         fp.semanticGalleryColumns(),
		Entries:                entries,
	}
}

func (fp *FileSystemPanel) semanticPagedPanelHeaderModel(
	ctx *vtui.SemanticContext, side int, active bool,
) (extui.PanelModel, bool) {
	if fp == nil || fp.vfs == nil {
		return extui.PanelModel{}, false
	}
	sourceKind, previewCapable := fp.semanticSourceInfo()
	fp.ensureSemanticPagedRevisions(sourceKind)
	panelID := vtui.SemanticID(fp)
	semanticLivePanels.Store(panelID, fp)
	cursor := fp.GetCursorIndex()
	cursorEntryID := ""
	if cursor >= 0 && cursor < len(fp.entries) && fp.entries[cursor] != nil {
		cursorEntryID, _ = fp.semanticEntryMetadata(fp.entries[cursor], sourceKind)
	}
	var fastFindMatches map[string]extui.FastFindMatchModel
	fastFindMatchColor := ""
	if first, end, active := fp.semanticFastFindRange(); active {
		fastFindMatchColor = semanticAttrColor(
			vtui.Palette[ColPanelHighlightText], true)
		fastFindMatches = fp.semanticFastFindMatches(
			first, end, func(index int) string {
				entry := fp.entries[index]
				if entry == nil {
					return ""
				}
				entryID, _ := fp.semanticEntryMetadata(entry, sourceKind)
				return entryID
			})
	}
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.galleryLayoutRevision
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	semanticTitle := fp.semanticTitle
	if semanticTitle == "" {
		semanticTitle = fp.currentTitle
	}
	return extui.PanelModel{
		ID: panelID, Side: side, Active: active, Path: fp.vfs.GetPath(),
		Title: semanticTitle, ShowFileInfo: AppConfig.ShowPanelFileInfo,
		GalleryLayoutMode:     string(galleryLayoutMode),
		GalleryColumnCount:    fp.effectiveGalleryColumnCount(),
		GalleryDensity:        fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:      fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision: galleryLayoutRevision,
		SourceKind:            sourceKind, PreviewCapable: previewCapable,
		CatalogRevision:   fp.catalogRevision,
		SelectionRevision: fp.selectionRevision,
		MetadataDeferred:  true, MetadataRevision: fp.metadataRevision,
		CatalogRowsDeferred: true,
		HighlightRevision:   semanticHighlighterRevision(),
		CursorEntryID:       cursorEntryID,
		SortMode:            sortModeName(fp.sortMode), SortReverse: fp.sortReverse,
		SeparateFileExtensions: AppConfig.SeparateFileExtensions,
		Cursor:                 cursor, Loading: fp.semanticLoading(),
		CatalogProvisional: fp.catalogProvisional,
		FastFind:           fp.fastFindMode, FastFindText: fp.fastFindStr,
		FastFindMatchColor: fastFindMatchColor,
		FastFindMatches:    fastFindMatches,
		SelectedCount:      len(fp.selectedItems), TotalCount: len(fp.entries),
		GalleryColumns: fp.semanticGalleryColumns(),
	}, true
}

// semanticPanelHeaderModel exports only the mutable, bounded panel state. The
// immutable catalog stays in the renderer/Qt cache until CatalogRevision
// changes. Returning false requests a complete fallback because no validated
// catalog has been published for this panel yet.
func (fp *FileSystemPanel) semanticPanelHeaderModel(ctx *vtui.SemanticContext, side int, active bool) (extui.PanelModel, bool) {
	if extUiPanelCatalogRowsIsEnabled() {
		return fp.semanticPagedPanelHeaderModel(ctx, side, active)
	}
	if fp == nil || !extUiPanelCatalogMetadataIsEnabled() {
		return extui.PanelModel{}, false
	}
	static := fp.semanticStaticCache
	if static == nil || static.catalogRevision != fp.catalogRevision {
		return extui.PanelModel{}, false
	}
	// During a cold directory transition Go has already changed the visible
	// path, but the real catalog is still being read. Keep the previously
	// published static catalog as the immutable base for this one provisional
	// state update. The native bridge intentionally keeps that old Gallery
	// session painted until the authoritative catalog_replace arrives; making
	// the header invalid here would fall back to a full scene and resend every
	// row of the other panel as well.
	provisionalReplacement := fp.catalogProvisional &&
		len(static.entries) != len(fp.entries)
	if len(static.entries) != len(fp.entries) && !provisionalReplacement {
		return extui.PanelModel{}, false
	}
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.galleryLayoutRevision
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	sourceKind, previewCapable := fp.semanticSourceInfo()
	cursor := fp.GetCursorIndex()
	cursorEntryID := ""
	if cursor >= 0 && cursor < len(static.entries) {
		cursorEntryID = static.entries[cursor].EntryID
	}
	var fastFindMatches map[string]extui.FastFindMatchModel
	fastFindMatchColor := ""
	if first, end, active := fp.semanticFastFindRange(); !provisionalReplacement && active {
		fastFindMatchColor = semanticAttrColor(vtui.Palette[ColPanelHighlightText], true)
		fastFindMatches = fp.semanticFastFindMatches(
			first, end, func(index int) string {
				return static.entries[index].EntryID
			})
	}
	semanticTitle := fp.semanticTitle
	if semanticTitle == "" {
		semanticTitle = fp.currentTitle
	}
	return extui.PanelModel{
		ID:                     vtui.SemanticID(fp),
		Side:                   side,
		Active:                 active,
		Path:                   fp.vfs.GetPath(),
		Title:                  semanticTitle,
		ShowFileInfo:           AppConfig.ShowPanelFileInfo,
		GalleryLayoutMode:      string(galleryLayoutMode),
		GalleryColumnCount:     fp.effectiveGalleryColumnCount(),
		GalleryDensity:         fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:       fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision:  galleryLayoutRevision,
		SourceKind:             sourceKind,
		PreviewCapable:         previewCapable,
		CatalogRevision:        fp.catalogRevision,
		SelectionRevision:      fp.selectionRevision,
		MetadataDeferred:       true,
		MetadataRevision:       fp.metadataRevision,
		HighlightRevision:      static.highlighterRevision,
		CursorEntryID:          cursorEntryID,
		SortMode:               sortModeName(fp.sortMode),
		SortReverse:            fp.sortReverse,
		SeparateFileExtensions: AppConfig.SeparateFileExtensions,
		Cursor:                 cursor,
		Loading:                fp.semanticLoading(),
		CatalogProvisional:     fp.catalogProvisional,
		FastFind:               fp.fastFindMode,
		FastFindText:           fp.fastFindStr,
		FastFindMatchColor:     fastFindMatchColor,
		FastFindMatches:        fastFindMatches,
		SelectedCount: func() int {
			if provisionalReplacement {
				return 0
			}
			return len(fp.selectedItems)
		}(),
		TotalCount:     len(static.entries),
		GalleryColumns: fp.semanticGalleryColumns(),
	}, true
}

// semanticGalleryColumns gives the reusable Details renderer a stable Name +
// Size schema independently from the terminal panel's current ViewMode.
func (fp *FileSystemPanel) semanticGalleryColumns() []extui.PanelColumnModel {
	width := fp.X2 - fp.X1 + 1
	nameWidth := width - 14
	if nameWidth < 5 {
		nameWidth = 5
	}
	title := func(base string, mode SortMode) string {
		if fp.sortMode != mode {
			return base
		}
		if fp.sortIsAscending() {
			return base + " ↑"
		}
		return base + " ↓"
	}
	return []extui.PanelColumnModel{
		{
			ID:        "name",
			Role:      "name",
			Index:     0,
			Title:     title(Msg("Panel.Column.Name"), SortName),
			Width:     nameWidth,
			Alignment: "left",
			SortMode:  sortModeName(SortName),
			Sortable:  true,
		},
		{
			ID:        "size",
			Role:      "size",
			Index:     1,
			Title:     title(Msg("Panel.Column.Size"), SortSize),
			Width:     panelSizeColumnWidth,
			Alignment: "right",
			SortMode:  sortModeName(SortSize),
			Sortable:  true,
		},
	}
}

func semanticFileSizeValue(entry *fileEntry) int64 {
	if entry == nil {
		return -1
	}
	// VFSItem.SizeKnown distinguishes a real empty file from the names/types
	// phase of a phased directory read. Preserve compatibility with providers
	// predating SizeKnown: every non-zero size was necessarily authoritative.
	if entry.SizeKnown || entry.Size != 0 || entry.SizeCalculated {
		return entry.Size
	}
	return -1
}

func semanticFileSize(entry *fileEntry) string {
	if entry == nil {
		return ""
	}
	if entry.IsDir {
		if entry.SizeCalculated {
			return formatIntWithSpaces(entry.Size)
		}
		if entry.Name == ".." {
			return Msg("Panel.UpDir")
		}
		return ""
	}
	size := semanticFileSizeValue(entry)
	if size < 0 {
		return ""
	}
	return formatIntWithSpaces(size)
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

func parseSortModeName(name string) (SortMode, bool) {
	switch name {
	case "name":
		return SortName, true
	case "extension":
		return SortExt, true
	case "time":
		return SortTime, true
	case "size":
		return SortSize, true
	case "unsorted":
		return SortUnsorted, true
	default:
		return SortUnsorted, false
	}
}

func (cl *CommandLine) semanticModel(ctx *vtui.SemanticContext) *extui.CommandLineModel {
	text := cl.Edit.GetText()
	cursorPosition, selectionStart, selectionEnd := semanticEditPositions(cl.Edit, text)
	model := &extui.CommandLineModel{
		ID:             vtui.SemanticID(cl),
		Visible:        cl.IsVisible(),
		Focused:        cl.IsFocused(),
		Prompt:         cl.Prompt,
		PromptRuns:     semanticRunsFromCells(cl.RichPrompt),
		Text:           text,
		Empty:          cl.IsEmpty(),
		InputX:         cl.Edit.X1 - cl.X1,
		CursorPosition: cursorPosition,
		SelectionStart: selectionStart,
		SelectionEnd:   selectionEnd,
	}
	rendered := semanticRenderSurface(cl.X1, cl.Y1, cl.X2, cl.Y2, cl.DisplayObject)
	if len(rendered.Rows) > 0 {
		model.Runs = rendered.Rows[0]
	}
	model.CursorX = rendered.CursorX
	model.CursorPrefixRuns = rendered.CursorPrefixRuns
	model.CursorVisible = rendered.CursorVisible
	model.CursorShape = rendered.CursorShape
	return model
}

// vtui.Edit keeps its logical caret and selection private because its normal
// renderer owns horizontal scrolling. The native QML command line instead
// needs those positions to give its TextInput the full window width. Keep this
// compatibility reader isolated here; if vtui adds public accessors, only this
// helper needs to change. QML positions use UTF-16 code units, while vtui uses
// rune indices.
func semanticEditPositions(edit *vtui.Edit, text string) (cursor, selectionStart, selectionEnd int) {
	runes := []rune(text)
	readRuneIndex := func(name string, fallback int) int {
		if edit == nil {
			return fallback
		}
		value := reflect.ValueOf(edit)
		if value.Kind() != reflect.Pointer || value.IsNil() {
			return fallback
		}
		field := value.Elem().FieldByName(name)
		if !field.IsValid() || field.Kind() != reflect.Int {
			return fallback
		}
		index := int(field.Int())
		if index < 0 {
			return index
		}
		if index > len(runes) {
			return len(runes)
		}
		return index
	}
	toUTF16 := func(runeIndex int) int {
		if runeIndex < 0 {
			return -1
		}
		return len(utf16.Encode(runes[:runeIndex]))
	}

	cursor = toUTF16(readRuneIndex("curPos", len(runes)))
	selectionStart = toUTF16(readRuneIndex("selStart", -1))
	selectionEnd = toUTF16(readRuneIndex("selEnd", -1))
	return cursor, selectionStart, selectionEnd
}

func (tv *TerminalView) semanticModel(ctx *vtui.SemanticContext) *extui.TerminalModel {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	buf := tv.Lines
	if tv.UseAltScreen {
		buf = tv.AltLines
	}
	offset := 0
	if !tv.UseAltScreen {
		lowestRow := 0
		for y := tv.Height - 1; y >= 0; y-- {
			if tv.rowHasText(y) {
				lowestRow = y
				break
			}
		}
		if tv.CursorY > lowestRow {
			lowestRow = tv.CursorY
		}
		if lowestRow < tv.Height-1 {
			offset = (tv.Height - 1) - lowestRow
		}
	}

	var rows []extui.TextRowModel
	for y := 0; y < tv.Height && y < len(buf); y++ {
		drawY := y + offset
		if tv.UseAltScreen {
			drawY = y
		}
		if drawY < 0 || drawY >= tv.Height {
			continue
		}
		rows = append(rows, extui.TextRowModel{
			Index: drawY,
			Runs:  semanticRunsFromCells(buf[y]),
		})
	}

	return &extui.TerminalModel{
		ID:        vtui.SemanticID(tv),
		Title:     tv.Title,
		Visible:   tv.IsVisible(),
		Focused:   tv.IsFocused(),
		AltScreen: tv.UseAltScreen,
		Busy:      tv.Muted,
		CursorX:   tv.CursorX,
		CursorY:   tv.CursorY + offset,
		Rows:      rows,
	}
}

func (vv *ViewerView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	if vv.semanticPendingScroll && vv.backend != nil {
		generation := vv.semanticPendingGeneration
		if generation <= vv.semanticWindowGeneration ||
			generation != vv.semanticWindowRequestGeneration {
			// A newer request already owns the viewport. A late cache fill for the
			// superseded seek must never move it backwards.
			vv.semanticPendingScroll = false
			vv.semanticPendingGeneration = 0
		} else if resolved, ready := vv.semanticResolveTextWindowOffset(vv.semanticPendingOffset); ready {
			vv.TopOffset = resolved
			vv.semanticPendingScroll = false
			vv.semanticPendingGeneration = 0
			vv.semanticWindowGeneration = generation
		}
	}
	window := vv.semanticWindow()
	width := vv.X2 - vv.X1 + 1
	if vv.scrollBar != nil {
		width--
	}
	window.rows = semanticStyledViewerWindowRows(vv, window, width)
	visibleEnd := min(window.viewportRow+window.viewportRows, len(window.rows))
	visibleRows := window.rows
	if window.viewportRow >= 0 && window.viewportRow <= visibleEnd {
		visibleRows = window.rows[window.viewportRow:visibleEnd]
	}
	mode := "text"
	if vv.HexMode {
		mode = "hex"
	}

	surface := extui.SurfaceModel{
		ID:                 vtui.SemanticID(vv),
		Kind:               "viewer",
		DefaultBackground:  semanticAttrColor(vtui.Palette[ColViewerText], false),
		Title:              vv.GetTitle(),
		Path:               vv.path,
		BaseName:           semanticBaseName(vv.vfs, vv.path),
		Mode:               mode,
		HexMode:            vv.HexMode,
		WrapMode:           vv.WrapMode,
		Busy:               vv.Busy,
		TopOffset:          vv.TopOffset,
		Size:               vv.backend.Size(),
		DocumentKey:        vtui.SemanticID(vv),
		ScrollAction:       "viewer.scrollWindow",
		ScrollUnit:         "bytes",
		WindowStart:        window.start,
		WindowEnd:          window.end,
		ViewportStart:      vv.TopOffset,
		ViewportSpan:       window.viewportSpan,
		ContentExtent:      vv.backend.Size(),
		ContentExtentKnown: true,
		ViewportRow:        window.viewportRow,
		ViewportRows:       window.viewportRows,
		WindowGeneration:   vv.semanticWindowGeneration,
		Rows:               visibleRows,
		WindowRows:         window.rows,
	}
	return surface.ToMap()
}

type semanticSurfaceWindow struct {
	rows         []extui.TextRowModel
	start        int64
	end          int64
	viewportRow  int
	viewportRows int
	viewportSpan int64
}

func semanticWindowBufferRows(viewportRows int) int {
	// Keep one complete viewport before and after the visible viewport.  The
	// native surface therefore has three screens of data and can continue a
	// high-velocity flick while the next bounded window is prepared.  This is
	// still O(viewport), independent of document size.
	return max(8, viewportRows)
}

func (vv *ViewerView) semanticContentWidth() int {
	width := vv.X2 - vv.X1 + 1
	if vv.scrollBar != nil {
		width--
	}
	return width
}

// semanticResolveTextWindowOffset maps an arbitrary byte position used by the
// GUI scrollbar back to the first byte of the visual viewer row containing it.
// In no-wrap mode a visual row is a logical line. In wrap mode it may begin in
// the middle of that line, so snapping only to the previous newline loses the
// actual viewport whenever a logical line occupies more than one screen row.
func (vv *ViewerView) semanticResolveTextWindowOffset(offset int64) (int64, bool) {
	offset = vv.clampTextScrollOffset(offset)
	if vv.backend == nil || offset <= 0 {
		return 0, true
	}
	if !vv.WrapMode {
		return vv.backend.TryFindLineStart(offset)
	}
	width := vv.semanticContentWidth()
	if width <= 0 {
		return vv.backend.TryFindLineStart(offset)
	}
	return vv.semanticWrappedRowStart(offset, width)
}

// clampTextScrollOffset keeps text-mode viewports inside the file.  EOF is a
// boundary, not a drawable byte, and resolving it as a line start can produce
// a blank page (especially for files ending in a newline).
func (vv *ViewerView) clampTextScrollOffset(offset int64) int64 {
	if vv.backend == nil || offset <= 0 {
		return 0
	}
	size := vv.backend.Size()
	if size <= 0 {
		return 0
	}
	if offset >= size {
		return size - 1
	}
	return offset
}

// semanticWrappedRowStart returns the visual fragment containing offset. All
// reads remain non-blocking: a cache miss leaves the previous complete window
// intact and is retried through semanticPendingScroll after ViewerBackend asks
// FrameManager for a redraw.
func (vv *ViewerView) semanticWrappedRowStart(offset int64, width int) (int64, bool) {
	seek := &vv.semanticWrapSeek
	historyLimit := semanticWindowBufferRows(max(1, vv.Y2-vv.Y1)) + 1
	if seek.width != width || len(seek.history) != historyLimit ||
		(!seek.active && !seek.ready) ||
		(seek.target != offset && (!seek.ready || offset < seek.resolved)) {
		seek.reset(offset, width, historyLimit)
	} else if seek.ready && seek.target == offset {
		return seek.resolved, true
	} else if seek.ready && offset == seek.resolved {
		seek.target = offset
		return seek.resolved, true
	} else if seek.ready && offset > seek.resolved {
		// A native scroll normally advances within the same large logical line.
		// Continue at the previously resolved fragment rather than searching and
		// scanning forward from that line's first byte again.
		seek.target = offset
		seek.active = true
		seek.ready = false
		seek.curr = seek.resolved
		seek.lineStartReady = true
	}

	if !seek.lineStartReady {
		lineStart, ready := vv.backend.TryFindLineStart(offset)
		if !ready {
			return offset, false
		}
		seek.curr = lineStart
		seek.lineStartReady = true
		seek.appendHistory(lineStart)
		if lineStart >= offset {
			seek.finish(lineStart)
			return lineStart, true
		}
	}
	if seek.curr >= offset {
		seek.finish(seek.curr)
		return seek.curr, true
	}

	for seek.curr < offset && seek.curr < vv.backend.Size() {
		data, err := vv.backend.ReadAt(seek.curr, width*4)
		if err == piecetable.ErrLoading {
			return offset, false
		}
		if err != nil || len(data) == 0 {
			seek.finish(seek.curr)
			return seek.curr, true
		}
		lineLen, _, _ := semanticViewerLineLen(data, width, true)
		if lineLen <= 0 {
			seek.finish(seek.curr)
			return seek.curr, true
		}
		nextOffset := min(seek.curr+int64(lineLen), vv.backend.Size())
		if offset < nextOffset {
			seek.finish(seek.curr)
			return seek.curr, true
		}
		if offset == nextOffset {
			seek.appendHistory(nextOffset)
			seek.finish(nextOffset)
			return nextOffset, true
		}
		if nextOffset <= seek.curr {
			seek.finish(seek.curr)
			return seek.curr, true
		}
		seek.curr = nextOffset
		seek.appendHistory(nextOffset)
	}
	seek.finish(seek.curr)
	return seek.curr, true
}

func (seek *semanticWrapSeekState) reset(target int64, width, historyLimit int) {
	history := seek.history
	if len(history) != historyLimit {
		history = make([]int64, historyLimit)
	}
	*seek = semanticWrapSeekState{
		active:  true,
		target:  target,
		width:   width,
		history: history,
	}
}

func (seek *semanticWrapSeekState) appendHistory(offset int64) {
	limit := len(seek.history)
	if limit == 0 {
		return
	}
	if seek.historyCount > 0 {
		last := (seek.historyHead + seek.historyCount - 1) % limit
		if seek.history[last] == offset {
			return
		}
	}
	if seek.historyCount < limit {
		index := (seek.historyHead + seek.historyCount) % limit
		seek.history[index] = offset
		seek.historyCount++
		return
	}
	seek.history[seek.historyHead] = offset
	seek.historyHead = (seek.historyHead + 1) % limit
}

func (seek *semanticWrapSeekState) finish(offset int64) {
	seek.curr = offset
	seek.resolved = offset
	seek.active = false
	seek.ready = true
	seek.appendHistory(offset)
}

func (seek *semanticWrapSeekState) previousHistoryOffset(offset int64, width int) (int64, bool) {
	if seek.width != width || seek.historyCount < 2 || len(seek.history) == 0 {
		return 0, false
	}
	for logicalIndex := seek.historyCount - 1; logicalIndex > 0; logicalIndex-- {
		index := (seek.historyHead + logicalIndex) % len(seek.history)
		if seek.history[index] != offset {
			continue
		}
		previous := (seek.historyHead + logicalIndex - 1) % len(seek.history)
		return seek.history[previous], true
	}
	return 0, false
}

// semanticPreviousTextRowStart steps back by one rendered row. WrapMode needs
// to walk the fragments of the containing logical line; FindLineStart alone
// would skip all of those fragments and jump directly to the previous '\n'.
func (vv *ViewerView) semanticPreviousTextRowStart(offset int64, width int) (int64, bool) {
	if offset <= 0 {
		return 0, true
	}
	if !vv.WrapMode || width <= 0 {
		return vv.backend.TryFindLineStart(offset - 1)
	}
	if previous, ok := vv.semanticWrapSeek.previousHistoryOffset(offset, width); ok {
		return previous, true
	}
	if vv.semanticWrapSeek.active && !vv.semanticWrapSeek.ready {
		// Do not reset ViewerBackend's persistent physical-line seek while a new
		// semantic window is still being resolved. The old QML window remains
		// usable until the generation acknowledgement arrives.
		return offset, false
	}

	lineStart, ready := vv.backend.TryFindLineStart(offset - 1)
	if !ready {
		return offset, false
	}
	currOffset := lineStart
	for currOffset < offset && currOffset < vv.backend.Size() {
		data, err := vv.backend.ReadAt(currOffset, width*4)
		if err == piecetable.ErrLoading {
			return offset, false
		}
		if err != nil || len(data) == 0 {
			return currOffset, true
		}
		lineLen, _, _ := semanticViewerLineLen(data, width, true)
		if lineLen <= 0 {
			return currOffset, true
		}
		nextOffset := min(currOffset+int64(lineLen), vv.backend.Size())
		if nextOffset >= offset || nextOffset <= currOffset {
			return currOffset, true
		}
		currOffset = nextOffset
	}
	return lineStart, true
}

func (vv *ViewerView) semanticWindow() semanticSurfaceWindow {
	var window semanticSurfaceWindow
	if vv.backend == nil {
		return window
	}
	width := vv.semanticContentWidth()
	contentHeight := vv.Y2 - vv.Y1
	if width <= 0 || contentHeight <= 0 {
		return window
	}
	window.viewportRows = contentHeight
	if vv.Busy {
		window.rows = []extui.TextRowModel{{Index: 0, Offset: vv.TopOffset,
			EndOffset: vv.TopOffset, Text: " [ Loading... ] "}}
		window.start, window.end = vv.TopOffset, vv.TopOffset
		return window
	}
	bufferRows := semanticWindowBufferRows(contentHeight)
	startOffset := vv.TopOffset
	if vv.HexMode {
		startOffset -= int64(bufferRows * 16)
		if startOffset < 0 {
			startOffset = 0
		}
		startOffset &= ^int64(0xF)
	} else {
		for i := 0; i < bufferRows && startOffset > 0; i++ {
			previous, ready := vv.semanticPreviousTextRowStart(startOffset, width)
			if !ready {
				break
			}
			if previous >= startOffset {
				break
			}
			startOffset = previous
		}
	}
	window.start = startOffset
	maxRows := contentHeight + 2*bufferRows
	if vv.HexMode {
		currOffset := startOffset &^ 0xF
		for y := 0; y < maxRows && currOffset < vv.backend.Size(); y++ {
			data, err := vv.backend.ReadAt(currOffset, 16)
			if err != nil && err != piecetable.ErrLoading {
				break
			}
			endOffset := min(currOffset+16, vv.backend.Size())
			window.rows = append(window.rows, extui.TextRowModel{
				Index:     y,
				Offset:    currOffset,
				EndOffset: endOffset,
				Text:      semanticHexLine(currOffset, data),
			})
			if currOffset == vv.TopOffset {
				window.viewportRow = y
			}
			currOffset = endOffset
		}
		window.end = currOffset
		visibleEnd := min(window.viewportRow+contentHeight, len(window.rows))
		if visibleEnd > window.viewportRow {
			window.viewportSpan = window.rows[visibleEnd-1].EndOffset - vv.TopOffset
		}
		return window
	}

	currOffset := startOffset
	for y := 0; y < maxRows; y++ {
		if currOffset >= vv.backend.Size() {
			break
		}
		data, err := vv.backend.ReadAt(currOffset, width*4)
		if err == piecetable.ErrLoading {
			window.rows = append(window.rows, extui.TextRowModel{Index: y,
				Offset: currOffset, EndOffset: currOffset, Text: " [ Loading... ] "})
			break
		}
		if err != nil || len(data) == 0 {
			break
		}
		lineLen, textLen, foundNewline := semanticViewerLineLen(data, width, vv.WrapMode)
		if lineLen <= 0 {
			break
		}
		nextOffset := min(currOffset+int64(lineLen), vv.backend.Size())
		if !vv.WrapMode && !foundNewline {
			// The displayed prefix is width-bounded, but one semantic row still
			// represents one complete logical line in no-wrap mode. Continue in
			// small bounded reads just like ViewerView.renderText.
			for nextOffset < vv.backend.Size() {
				chunk, readErr := vv.backend.ReadAt(nextOffset, 1024)
				if readErr != nil || len(chunk) == 0 {
					break
				}
				newline := -1
				for i, char := range chunk {
					if char == '\n' {
						newline = i
						break
					}
				}
				if newline >= 0 {
					nextOffset += int64(newline + 1)
					break
				}
				nextOffset += int64(len(chunk))
			}
		}
		window.rows = append(window.rows, extui.TextRowModel{Index: y,
			Offset: currOffset, EndOffset: nextOffset, Text: string(data[:textLen])})
		if currOffset == vv.TopOffset {
			window.viewportRow = y
		}
		currOffset = nextOffset
	}
	window.end = currOffset
	visibleEnd := min(window.viewportRow+contentHeight, len(window.rows))
	if visibleEnd > window.viewportRow {
		window.viewportSpan = window.rows[visibleEnd-1].EndOffset - vv.TopOffset
	}
	return window
}

func semanticHexLine(offset int64, data []byte) string {
	hexPart := ""
	asciiPart := ""
	for i := 0; i < 16; i++ {
		if i < len(data) {
			hexPart += fmt.Sprintf("%02X ", data[i])
			r := rune(data[i])
			if r < 32 || r > 126 {
				r = '.'
			}
			asciiPart += string(r)
		} else {
			hexPart += "   "
		}
		if i == 7 {
			hexPart += " "
		}
	}
	return fmt.Sprintf("%010X: %s | %s", offset, hexPart, asciiPart)
}

func semanticViewerLineLen(data []byte, width int, wrap bool) (lineLen int, textLen int, foundNewline bool) {
	visualWidth := 0
	tabSize := 8
	if AppConfig.EditorTabSize > 0 {
		tabSize = AppConfig.EditorTabSize
	}
	for lineLen < len(data) {
		r, size := utf8.DecodeRune(data[lineLen:])
		if r == '\n' {
			lineLen += size
			return lineLen, textLen, true
		}
		if r == '\r' {
			lineLen += size
			continue
		}
		rw := 1
		if r == '\t' {
			rw = tabSize - (visualWidth % tabSize)
		} else {
			rw = runewidth.RuneWidth(r)
			if rw <= 0 {
				rw = 1
			}
		}
		if wrap && visualWidth+rw > width {
			return lineLen, textLen, false
		}
		visualWidth += rw
		lineLen += size
		if wrap || visualWidth <= width {
			textLen = lineLen
		}
	}
	return lineLen, textLen, false
}

type semanticEditorCursorState struct {
	line         int
	pos          int
	visualRow    int
	visualColumn int
	visible      bool
	shape        string
	absoluteRow  int64
}

func (state semanticEditorCursorState) ToMap() map[string]any {
	return map[string]any{
		"cursorLine":         state.line,
		"cursorPos":          state.pos,
		"cursorVisualRow":    state.visualRow,
		"cursorVisualColumn": state.visualColumn,
		"cursorVisible":      state.visible,
		"cursorShape":        state.shape,
		"cursorAbsoluteRow":  state.absoluteRow,
	}
}

func (ev *EditorView) semanticSurfaceWidth() int {
	width := ev.X2 - ev.X1 + 1
	if ev.scrollBar != nil {
		width--
	}
	return max(0, width)
}

func (ev *EditorView) semanticCursorState(width int) semanticEditorCursorState {
	cursorOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	cursorAbsoluteRow, cursorAbsoluteColumn := ev.engine.LogicalToVisual(cursorOffset)
	cursorVisualColumn := cursorAbsoluteColumn + ev.CursorVirtualSpaces - ev.ScrollLeft
	cursorVisible := ev.IsVisible() && !ev.pasting && !ev.saving &&
		cursorVisualColumn >= 0 && cursorVisualColumn < width
	cursorShape := "underline"
	if ev.overtype {
		cursorShape = "block"
	}
	return semanticEditorCursorState{
		line:         ev.CursorLine,
		pos:          ev.CursorPos,
		visualRow:    cursorAbsoluteRow - ev.ScrollTopRow,
		visualColumn: cursorVisualColumn,
		visible:      cursorVisible,
		shape:        cursorShape,
		absoluteRow:  int64(cursorAbsoluteRow),
	}
}

func (ev *EditorView) queueSemanticCursorState() bool {
	if ev == nil || ev.li == nil || ev.engine == nil || vtui.FrameManager == nil {
		return false
	}
	screen := vtui.FrameManager.Screen()
	if screen == nil || screen.Renderer == nil {
		return false
	}
	renderer, ok := screen.Renderer.(interface {
		QueueSurfaceState(string, map[string]any) bool
	})
	if !ok {
		return false
	}
	state := ev.semanticCursorState(ev.semanticSurfaceWidth())
	return renderer.QueueSurfaceState(vtui.SemanticID(ev), state.ToMap())
}

func (ev *EditorView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	window := ev.semanticWindow()
	width := ev.semanticSurfaceWidth()
	window.rows = semanticStyledEditorWindowRows(ev, window, width)
	visibleEnd := min(window.viewportRow+window.viewportRows, len(window.rows))
	visibleRows := window.rows
	if window.viewportRow >= 0 && window.viewportRow <= visibleEnd {
		visibleRows = window.rows[window.viewportRow:visibleEnd]
	}
	cursor := ev.semanticCursorState(width)

	surface := extui.SurfaceModel{
		ID:                 vtui.SemanticID(ev),
		Kind:               "editor",
		DefaultBackground:  semanticAttrColor(ColorerEditorBaseAttr(vtui.Palette[ColEditorText]), false),
		Title:              ev.GetTitle(),
		Path:               ev.filePath,
		BaseName:           semanticBaseName(ev.vfs, ev.filePath),
		Busy:               ev.IsBusy(),
		Dirty:              ev.modified,
		Saving:             ev.saving,
		WordWrap:           ev.WordWrap,
		Overtype:           ev.overtype,
		CursorLine:         cursor.line,
		CursorPos:          cursor.pos,
		CursorVisualRow:    cursor.visualRow,
		CursorVisualColumn: cursor.visualColumn,
		CursorVisible:      cursor.visible,
		CursorShape:        cursor.shape,
		ScrollTop:          ev.ScrollTopRow,
		ScrollLeft:         ev.ScrollLeft,
		DocumentKey:        vtui.SemanticID(ev),
		ScrollAction:       "editor.scroll",
		ScrollUnit:         "rows",
		WindowStart:        window.start,
		WindowEnd:          window.end,
		ViewportStart:      int64(ev.ScrollTopRow),
		ViewportSpan:       window.viewportSpan,
		ContentExtent:      int64(ev.engine.GetTotalVisualRows()),
		ContentExtentKnown: ev.semanticExtentKnown,
		ViewportRow:        window.viewportRow,
		ViewportRows:       window.viewportRows,
		CursorAbsoluteRow:  cursor.absoluteRow,
		WindowGeneration:   ev.semanticWindowGeneration,
		Selection:          ev.selActive,
		Rows:               visibleRows,
		WindowRows:         window.rows,
		Autocomplete:       ev.semanticAutocomplete(),
	}
	return surface.ToMap()
}

// GetText возвращает текущий текст редактора из PieceTable
func (ev *EditorView) GetText() string {
	if ev.pt == nil {
		return ""
	}
	return ev.pt.String()
}

// HandleSemanticAction обрабатывает нативные GUI-действия для EditorView
func (ev *EditorView) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if vtui.SemanticID(ev) != target {
		return false
	}

	switch semanticString(action["action"]) {
	case "editor.setText":
		text := semanticString(action["text"])
		ev.SetText(text)
		return true
	case "editor.insertText":
		text := semanticString(action["text"])
		ev.PasteText(text)
		return true
	case "editor.deleteSelection":
		ev.DeleteSelection()
		return true
	case "editor.undo":
		ev.Undo()
		return true
	case "editor.redo":
		ev.Redo()
		return true
	case "editor.save":
		ev.SaveToFile(nil)
		return true
	case "editor.search":
		pattern := semanticString(action["pattern"])
		caseSensitive := semanticBool(action["case"])
		reverse := semanticBool(action["reverse"])
		next := semanticBool(action["next"])
		ev.Search(pattern, caseSensitive, reverse, false, false, next)
		return true
	case "editor.mouse":
		buttonState := uint32(0)
		switch semanticString(action["button"]) {
		case "left":
			buttonState = vtinput.FromLeft1stButtonPressed
		case "right":
			buttonState = vtinput.RightmostButtonPressed
		case "middle":
			buttonState = vtinput.FromLeft2ndButtonPressed
		}
		flags := uint32(0)
		if semanticBool(action["moved"]) {
			flags |= vtinput.MouseMoved
		}
		if semanticBool(action["doubleClick"]) {
			flags |= vtinput.DoubleClick
		}
		controlState := vtinput.ControlKeyState(0)
		if semanticBool(action["shift"]) {
			controlState |= vtinput.ShiftPressed
		}
		if semanticBool(action["ctrl"]) {
			controlState |= vtinput.LeftCtrlPressed
		}
		if semanticBool(action["alt"]) {
			controlState |= vtinput.LeftAltPressed
		}
		column := max(0, semanticInt(action["column"]))
		row := max(0, semanticInt(action["row"]))
		event := &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          int16(min(ev.X2, ev.X1+column)),
			MouseY:          int16(min(ev.Y2, ev.Y1+1+row)),
			ButtonState:     buttonState,
			MouseEventFlags: flags,
			WheelDirection:  semanticInt(action["wheelDirection"]),
			KeyDown:         semanticString(action["phase"]) != "release",
			ControlKeyState: controlState,
			InputSource:     "semantic",
		}
		ev.ProcessMouse(event)
		return true
	case "editor.scroll":
		generation, accepted := semanticAcceptWindowGeneration(action,
			ev.semanticWindowGeneration, &ev.semanticWindowRequestGeneration)
		if !accepted {
			return true
		}
		ev.ensureEngineWidth()
		top := semanticInt(action["visualRow"])
		height := max(1, ev.Y2-ev.Y1)
		maxTop := max(0, ev.engine.GetTotalVisualRows()-height)
		if top < 0 {
			top = 0
		}
		if top > maxTop {
			top = maxTop
		}
		ev.ScrollTopRow = top
		ev.semanticWindowGeneration = generation
		return true
	case "control.focus":
		ev.SetFocus(true)
		return true
	}
	return false
}

func (ev *EditorView) semanticWindow() semanticSurfaceWindow {
	var window semanticSurfaceWindow
	if ev.pt == nil || ev.li == nil || ev.engine == nil {
		return window
	}
	ev.ensureEngineWidth()
	height := ev.Y2 - ev.Y1
	if height <= 0 {
		return window
	}
	window.viewportRows = height
	bufferRows := semanticWindowBufferRows(height)
	windowStart := max(0, ev.ScrollTopRow-bufferRows)
	totalRows := ev.engine.GetTotalVisualRows()
	windowEnd := min(totalRows, ev.ScrollTopRow+height+bufferRows)
	window.start = int64(windowStart)
	window.end = int64(windowEnd)
	window.viewportRow = ev.ScrollTopRow - windowStart
	window.viewportSpan = int64(min(height, max(0, totalRows-ev.ScrollTopRow)))
	startLogLine, startFragIdx := ev.engine.GetLogLineAtVisualRow(windowStart)
	for logIdx := startLogLine; logIdx < ev.li.LineCount() && len(window.rows) < windowEnd-windowStart; logIdx++ {
		frags := ev.engine.GetFragments(logIdx)
		baseVRow := ev.engine.GetRowOffset(logIdx)
		for fIdx, frag := range frags {
			if logIdx == startLogLine && fIdx < startFragIdx {
				continue
			}
			data, err := ev.pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
			text := string(data)
			if err == piecetable.ErrLoading {
				text = " [ Loading... ] "
			} else if err != nil {
				text = ""
			}
			visualRow := baseVRow + fIdx
			if visualRow >= windowEnd {
				break
			}
			window.rows = append(window.rows, extui.TextRowModel{
				Index:       len(window.rows),
				VisualRow:   visualRow,
				LogicalLine: logIdx,
				Offset:      int64(frag.ByteOffsetStart),
				EndOffset:   int64(frag.ByteOffsetEnd),
				Text:        text,
			})
			if len(window.rows) >= windowEnd-windowStart {
				break
			}
		}
	}
	return window
}

func (ev *EditorView) semanticAutocomplete() map[string]any {
	if !ev.acEnabled || len(ev.acMatches) == 0 || ev.acCurrentIdx < 0 || ev.acCurrentIdx >= len(ev.acMatches) {
		return nil
	}
	match := ev.acMatches[ev.acCurrentIdx]
	if len(match) <= len(ev.acPrefix) {
		return nil
	}
	return map[string]any{
		"prefix": ev.acPrefix,
		"tail":   match[len(ev.acPrefix):],
		"index":  ev.acCurrentIdx,
	}
}

func semanticRunsFromCells(cells []vtui.CharInfo) []extui.RunModel {
	if len(cells) == 0 {
		return nil
	}
	var runs []extui.RunModel
	var b strings.Builder
	var attr uint64
	haveRun := false
	flush := func() {
		if !haveRun {
			return
		}
		runs = append(runs, extui.RunModel{
			Text:       b.String(),
			Attr:       attr,
			Foreground: semanticAttrColor(attr, true),
			Background: semanticAttrColor(attr, false),
			Bold:       attr&vtui.ForegroundIntensity != 0,
			Underline:  attr&vtui.CommonLvbUnderscore != 0,
			Strikeout:  attr&vtui.CommonLvbStrikeout != 0,
		})
		b.Reset()
	}
	for _, cell := range cells {
		if cell.Char == vtui.WideCharFiller {
			continue
		}
		ch := cellRune(cell.Char)
		if !haveRun {
			attr = cell.Attributes
			haveRun = true
		} else if cell.Attributes != attr {
			flush()
			attr = cell.Attributes
			haveRun = true
		}
		b.WriteRune(ch)
	}
	flush()
	return runs
}

type semanticRenderedSurface struct {
	Rows             [][]extui.RunModel
	CursorPrefixRuns []extui.RunModel
	CursorX          int
	CursorY          int
	CursorVisible    bool
	CursorShape      string
}

func semanticRenderSurface(x1, y1, x2, y2 int, render func(*vtui.ScreenBuf)) semanticRenderedSurface {
	result := semanticRenderedSurface{CursorX: -1, CursorY: -1}
	if render == nil || x2 < x1 || y2 < y1 {
		return result
	}
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(max(1, x2+1), max(1, y2+1))
	scr.ThemePalette = &vtui.ThemePalette
	render(scr)
	cursorX, cursorY, visible, shape := scr.GetCursorStateForTesting()
	result.Rows = make([][]extui.RunModel, 0, y2-y1+1)
	for y := y1; y <= y2; y++ {
		cells := make([]vtui.CharInfo, 0, x2-x1+1)
		for x := x1; x <= x2; x++ {
			cells = append(cells, scr.GetCell(x, y))
		}
		result.Rows = append(result.Rows, semanticRunsFromCells(cells))
		if visible && y == cursorY && cursorX >= x1 && cursorX <= x2 {
			prefixLength := min(cursorX-x1, len(cells))
			result.CursorPrefixRuns = semanticRunsFromCells(cells[:prefixLength])
		}
	}
	if visible && cursorX >= x1 && cursorX <= x2 && cursorY >= y1 && cursorY <= y2 {
		result.CursorX = cursorX - x1
		result.CursorY = cursorY - y1
		result.CursorVisible = true
		switch shape {
		case vtui.CursorShapeBlock:
			result.CursorShape = "block"
		default:
			result.CursorShape = "underline"
		}
	}
	return result
}

func semanticRowsWithRenderedRunsAt(rows []extui.TextRowModel,
	rendered [][]extui.RunModel, firstRow int) []extui.TextRowModel {
	if len(rendered) == 0 || len(rows) == 0 {
		return rows
	}
	result := append([]extui.TextRowModel(nil), rows...)
	for index, runs := range rendered {
		rowIndex := firstRow + index
		if rowIndex < 0 || rowIndex >= len(result) {
			continue
		}
		result[rowIndex].Text = ""
		result[rowIndex].Runs = runs
	}
	return result
}

func semanticAttrColor(attr uint64, foreground bool) string {
	reverse := attr&vtui.CommonLvbReverse != 0
	if reverse {
		foreground = !foreground
	}
	var rgb uint32
	if foreground {
		if attr&vtui.IsFgRGB != 0 {
			rgb = vtui.GetRGBFore(attr)
		} else {
			rgb = vtui.ThemePalette[vtui.GetIndexFore(attr)]
		}
		if attr&vtui.ForegroundDim != 0 {
			rgb = ((rgb>>16&0xff)/2)<<16 | ((rgb>>8&0xff)/2)<<8 | (rgb&0xff)/2
		}
	} else if attr&vtui.IsBgRGB != 0 {
		rgb = vtui.GetRGBBack(attr)
	} else {
		rgb = vtui.ThemePalette[vtui.GetIndexBack(attr)]
	}
	return fmt.Sprintf("#%06x", rgb&0xffffff)
}

func cellRune(ch uint64) rune {
	if ch == 0 || ch > utf8.MaxRune || (ch >= 0xD800 && ch <= 0xDFFF) {
		return ' '
	}
	return rune(ch)
}

func semanticBaseName(v interface{ Base(string) string }, path string) string {
	if path == "" {
		return ""
	}
	if v != nil {
		return v.Base(path)
	}
	return filepath.Base(path)
}

func semanticString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// semanticAcceptWindowGeneration turns the native viewport's client sequence
// into an acknowledgement token.  QML may keep moving over the old overscan
// while a request is being resolved asynchronously, so an older request can
// arrive after a newer one.  Such a request is handled (the caller need not
// retry it), but it is not allowed to mutate the canonical viewport.
//
// generation is optional for compatibility with terminal/legacy semantic
// callers.  Those callers receive a monotonically allocated generation too.
func semanticAcceptWindowGeneration(action map[string]any, acknowledged uint64,
	highWater *uint64,
) (generation uint64, accepted bool) {
	latest := acknowledged
	if highWater != nil && *highWater > latest {
		latest = *highWater
	}
	if raw, present := action["generation"]; present {
		requested := semanticInt64(raw)
		if requested <= 0 {
			return 0, false
		}
		generation = uint64(requested)
		if generation <= latest {
			return generation, false
		}
	} else {
		if latest == ^uint64(0) {
			return latest, false
		}
		generation = latest + 1
	}
	if highWater != nil {
		*highWater = generation
	}
	return generation, true
}

func semanticInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func semanticInt64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		if n > uint64(^uint64(0)>>1) {
			return 0
		}
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

func semanticStringSlice(v any) []string {
	switch values := v.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

func semanticIntSlice(v any) []int {
	switch values := v.(type) {
	case []int:
		return append([]int(nil), values...)
	case []any:
		out := make([]int, 0, len(values))
		for _, value := range values {
			out = append(out, semanticInt(value))
		}
		return out
	}
	return nil
}

func semanticBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if n, ok := v.(int); ok {
		return n != 0
	}
	if f, ok := v.(float64); ok {
		return f != 0
	}
	return false
}
