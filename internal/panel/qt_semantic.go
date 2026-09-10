package panel

import (
	"crypto/sha256"
	"fmt"
	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/media"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"hash/fnv"
	"os"
	"os/user"
	"sort"
	"strings"
	"sync"
)

func (pf *PanelsFrame) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          strings.TrimSpace(pf.GetTitle()),
		Mode:           "panels",
		ActivePanel:    pf.ActiveIdx,
		ShowPanels:     pf.ShowPanels,
		ShowLeftPanel:  pf.ShowLeftPanel,
		ShowRightPanel: pf.ShowRightPanel,
		Wide:           pf.Wide,
		WidePanel:      pf.WidePanel,
		PanelLayout:    pf.semanticPanelLayoutModel(ctx),
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
		if info, ok := pf.AltPanels[i].(*InfoPanel); ok {
			shell.InfoPanels = append(shell.InfoPanels, info.semanticModel(i, i == pf.ActiveIdx))
		}
		if quick, ok := pf.AltPanels[i].(*QuickViewPanel); ok {
			sourceSide := -1
			for candidate, source := range pf.Panels {
				if source == quick.Source() {
					sourceSide = candidate
					break
				}
			}
			shell.QuickViews = append(shell.QuickViews,
				quick.semanticModel(i, sourceSide, i == pf.ActiveIdx))
		}
	}

	if pf.CmdLine != nil {
		shell.CommandLine = pf.CmdLine.SemanticModel(ctx)
	}
	if pf.TermView != nil {
		shell.Terminal = pf.TermView.SemanticModelWithBottomOverlay(
			ctx, terminalCommandLineOverlayRows(shell.CommandLine))
	}
	if macro.MacroMgr != nil && macro.MacroMgr.Recording {
		shell.MacroRecording = true
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		shell.Fallback = true
		shell.FallbackReason = reason
	}

	return shell.ToMap()
}

func terminalCommandLineOverlayRows(commandLine *extui.CommandLineModel) int {
	if commandLine != nil && commandLine.Visible {
		return 1
	}
	return 0
}

func (pf *PanelsFrame) semanticPanelLayoutModel(ctx *vtui.SemanticContext) extui.PanelLayoutModel {
	columns := pf.LastW
	if ctx != nil && ctx.Width > 0 {
		columns = ctx.Width
	}
	if columns < 0 {
		columns = 0
	}

	widthDecrement := pf.WidthDecrement
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
		height := pf.LastH
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
		LeftBottomInsetRows:  clampBottomInset(pf.LeftHeightDecrement),
		RightBottomInsetRows: clampBottomInset(pf.RightHeightDecrement),
	}
}

func (pf *PanelsFrame) semanticGridFallbackReason() string {
	for _, panel := range pf.AltPanels {
		if panel != nil && panel.Kind() != "info" && panel.Kind() != "quick_view" {
			return "the QML presentation does not yet support the " + panel.Kind() + " panel"
		}
	}
	return ""
}

func (ip *InfoPanel) semanticModel(side int, active bool) extui.InfoPanelModel {
	model := extui.InfoPanelModel{
		ID:         vtui.SemanticID(ip),
		Side:       side,
		Active:     active,
		Title:      i18n.Msg("InfoPanel.Title"),
		BottomHint: i18n.Msg("InfoPanel.UnitsHint"),
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
	row(i18n.Msg("InfoPanel.Computer"), hostname)
	row(i18n.Msg("InfoPanel.User"), username)
	blank()

	path := ""
	if ip.src != nil && ip.src.Vfs != nil {
		path = ip.src.Vfs.GetPath()
	}
	fsTitle := i18n.Msg("InfoPanel.FilesystemTitle")
	if fs, ok := sysinfo.FS(path); ok {
		if fs.Type != "" {
			fsTitle = fmt.Sprintf("%s (%s)", fsTitle, fs.Type)
		}
		section(fsTitle)
		row(i18n.Msg("InfoPanel.Total"), formatBytes(fs.Total))
		row(i18n.Msg("InfoPanel.Free"), formatBytes(fs.Free))
		if fs.Label != "" {
			row(i18n.Msg("InfoPanel.Label"), fs.Label)
		}
		if fs.Serial != "" {
			row(i18n.Msg("InfoPanel.Serial"), fs.Serial)
		}
		row(i18n.Msg("InfoPanel.CurrentDir"), path)
		if fs.Mount != "" && fs.Mount != path {
			row(i18n.Msg("InfoPanel.Mount"), fs.Mount)
		}
		if fs.MaxFilename > 0 {
			row(i18n.Msg("InfoPanel.MaxFilename"), fmt.Sprintf("%d", fs.MaxFilename))
		}
		if fs.Flags != "" {
			row(i18n.Msg("InfoPanel.Flags"), fs.Flags)
		}
	} else {
		section(fsTitle)
		row(i18n.Msg("InfoPanel.CurrentDir"), path)
	}

	if mem, ok := sysinfo.Mem(); ok {
		blank()
		section(i18n.Msg("InfoPanel.MemoryTitle"))
		row(i18n.Msg("InfoPanel.MemLoad"), fmt.Sprintf("%d%%", mem.LoadPercent))
		row(i18n.Msg("InfoPanel.MemTotal"), formatBytes(mem.Total))
		row(i18n.Msg("InfoPanel.MemFree"), formatBytes(mem.Free))
		if mem.Shared > 0 {
			row(i18n.Msg("InfoPanel.MemShared"), formatBytes(mem.Shared))
		}
		if mem.Buffered > 0 {
			row(i18n.Msg("InfoPanel.MemBuffered"), formatBytes(mem.Buffered))
		}
		if mem.SwapTotal > 0 {
			row(i18n.Msg("InfoPanel.SwapTotal"), formatBytes(mem.SwapTotal))
			row(i18n.Msg("InfoPanel.SwapFree"), formatBytes(mem.SwapFree))
		}
	}
	return model
}

func (pf *PanelsFrame) HandleSemanticAction(action map[string]any) bool {
	if pf.TermView != nil && pf.TermView.HandleSemanticAction(action) {
		return true
	}
	if semantic.String(action["action"]) == "quickView.scroll" {
		for _, panel := range pf.AltPanels {
			if quick, ok := panel.(*QuickViewPanel); ok && quick.HandleSemanticAction(action) {
				return true
			}
		}
		return false
	}
	switch semantic.String(action["action"]) {
	case "panel.dropFiles":
		return pf.handleSemanticDrop(action)
	case "activate_panel", "panel.activate":
		side := semantic.Int(action["side"])
		if side >= 0 && side < len(pf.Panels) {
			pf.setActivePanelForAction(action)
			pf.LastKey = 0
			if fsp, ok := pf.Panels[side].(*FileSystemPanel); ok {
				fsp.clearFastFindForSemanticPointerIntent()
			}
			return true
		}
	case "panel_navigate_path", "panel.navigatePath":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil || fsp.Vfs == nil {
			return false
		}
		if benchmark := navtrace.NavigationBenchmarkCurrentUI(); benchmark != nil {
			benchmark.SetSide(pf.panelIndexForSemanticAction(action))
		}
		target := strings.TrimSpace(semantic.String(action["path"]))
		if target == "" {
			return false
		}
		oldPath := fsp.Vfs.GetPath()
		if target == oldPath {
			return true
		}
		if err := fsp.SetKnownDirectoryPath(target); err != nil {
			fsp.showDirectoryError(" Error ", fmt.Sprintf("Cannot access folder:\n%v", err))
			return true
		}
		pf.setActivePanelForAction(action)
		fsp.clearFastFindForSemanticPointerIntent()
		fsp.PendingSelection = fsp.Vfs.Base(oldPath)
		fsp.ReadDirectory()
		return true
	case "panel_drive_menu", "panel.driveMenu":
		side := pf.panelIndexForSemanticAction(action)
		fsp := pf.panelForSemanticAction(action)
		if side < 0 || fsp == nil {
			return false
		}
		pf.setActivePanelForAction(action)
		pf.LastKey = 0
		fsp.clearFastFindForSemanticPointerIntent()
		allowSemanticMenuAfterPanelActivation()
		pf.ShowDriveMenu(side)
		return true
	case "panel_cursor", "panel.cursor":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			activated := false
			// A native GUI row click targets an item and activates its panel as
			// one visual operation. Applying those intents atomically prevents
			// an intermediate scene from highlighting the panel's old cursor.
			if semantic.Bool(action["activate"]) {
				pf.setActivePanelForAction(action)
				pf.LastKey = 0
				activated = true
			}
			idx, ok := fsp.semanticEntryIndex(action)
			if !ok {
				return activated
			}
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetCursorIndex(idx)
			return true
		}
	case "panel_pointer", "panel.pointer":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			phase := semantic.String(action["phase"])
			button := semantic.String(action["button"])
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
				pf.LastKey = 0
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
			if benchmark := navtrace.NavigationBenchmarkCurrentUI(); benchmark != nil {
				benchmark.SetSide(pf.panelIndexForSemanticAction(action))
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
			if rawRevision, present := action["selectionRevision"]; present && semantic.Int64(rawRevision) != fsp.selectionRevision {
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
			if semantic.Bool(action["activate"]) {
				pf.setActivePanelForAction(action)
				pf.LastKey = 0
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
		if semantic.Bool(enabled) {
			pf.SetWidePanel(side)
		} else if pf.Wide && pf.WidePanel == side {
			pf.ExitWide()
		} else {
			return false
		}
		return true
	case "panel_set_gallery_layout", "panel.setGalleryLayout":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semantic.String(action["layoutMode"])
		if modeValue == "" {
			modeValue = semantic.String(action["galleryLayoutMode"])
		}
		mode, ok := ParseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		columns := semantic.Int(action["columnCount"])
		if columns == 0 {
			columns = fsp.effectiveGalleryColumnCount()
		}
		previousRevision := fsp.GalleryLayoutRevision
		if !fsp.SetGalleryLayout(mode, columns) {
			return false
		}
		pf.UpdateMenuCheckmarks()
		if fsp.GalleryLayoutRevision != previousRevision {
			persistNativePanelLayoutSession(pf)
		}
		return true
	case "panel_set_gallery_density", "panel.setGalleryDensity":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semantic.String(action["layoutMode"])
		if modeValue == "" {
			modeValue = semantic.String(action["galleryLayoutMode"])
		}
		if modeValue == "" {
			modeValue = string(fsp.effectiveGalleryLayoutMode())
		}
		mode, ok := ParseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		previousRevision := fsp.GalleryLayoutRevision
		if !fsp.SetGalleryDensity(mode, semantic.Int(action["density"])) {
			return false
		}
		if fsp.GalleryLayoutRevision != previousRevision {
			persistNativePanelLayoutSession(pf)
		}
		return true
	case "panel_reset_gallery_density", "panel.resetGalleryDensity":
		fsp := pf.panelForSemanticAction(action)
		if fsp == nil {
			return false
		}
		modeValue := semantic.String(action["layoutMode"])
		if modeValue == "" {
			modeValue = semantic.String(action["galleryLayoutMode"])
		}
		if modeValue == "" {
			modeValue = string(fsp.effectiveGalleryLayoutMode())
		}
		mode, ok := ParseGalleryLayoutMode(modeValue)
		if !ok {
			return false
		}
		previousRevision := fsp.GalleryLayoutRevision
		if !fsp.ResetGalleryDensity(mode) {
			return false
		}
		if fsp.GalleryLayoutRevision != previousRevision {
			persistNativePanelLayoutSession(pf)
		}
		return true
	case "panel.sortGroups":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			enabled, ok := action["enabled"].(bool)
			if !ok {
				return false
			}
			pf.setActivePanelForAction(action)
			fsp.SetUseSortGroups(enabled)
			pf.UpdateMenuCheckmarks()
			return true
		}
	case "panel_sort", "panel.sort":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			mode, ok := parseSortModeName(semantic.String(action["mode"]))
			if !ok {
				return false
			}
			pf.setActivePanelForAction(action)
			pf.LastKey = 0
			fsp.clearFastFindForSemanticPointerIntent()
			fsp.SetSortMode(mode)
			pf.UpdateMenuCheckmarks()
			return true
		}
	case "panel_sort_menu", "panel.sortMenu":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			pf.setActivePanelForAction(action)
			pf.LastKey = 0
			fsp.clearFastFindForSemanticPointerIntent()
			allowSemanticMenuAfterPanelActivation()
			SortMenuForPanel(pf, fsp)
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
		cmdline.CloseActiveAutocompleteMenus()
		return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "complete_command", "command.complete":
		// QML keeps autocomplete selection local so arrow-key repeats never
		// wait for a semantic-scene round trip.  Tab commits that explicit
		// selection, if any, but an untouched menu merely closes and preserves
		// the exact command line.
		if text, present := action["text"]; present && pf.CmdLine != nil {
			pf.CmdLine.Edit.SetText(semantic.String(text))
		}
		cmdline.CloseActiveAutocompleteMenus()
		return true
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
	side := pf.panelIndexForSemanticAction(action)
	if side >= 0 && side < len(pf.Panels) {
		pf.switchActivePanel(side)
	}
}

func (pf *PanelsFrame) panelIndexForSemanticAction(action map[string]any) int {
	side := pf.ActiveIdx
	if _, present := action["side"]; present {
		side = semantic.Int(action["side"])
	}
	if side < 0 || side >= len(pf.Panels) {
		return -1
	}
	return side
}

func (pf *PanelsFrame) panelForSemanticAction(action map[string]any) *FileSystemPanel {
	side := pf.panelIndexForSemanticAction(action)
	if side < 0 {
		return nil
	}
	if fsp, ok := pf.Panels[side].(*FileSystemPanel); ok {
		return fsp
	}
	return nil
}

func (fp *FileSystemPanel) clearFastFindForSemanticPointerIntent() {
	if !fp.FastFindMode && fp.FastFindStr == "" {
		return
	}
	fp.FastFindMode = false
	fp.FastFindStr = ""
	// Fast Find is panel state, so neither a scalar panel activation nor a
	// root-only menu patch can represent this mutation. Fall back to the
	// authoritative incremental/full scene for this uncommon transition.
	invalidateSemanticSceneUpdate()
}

func (fp *FileSystemPanel) semanticEntryIndex(action map[string]any) (idx int, ok bool) {
	benchmark := navtrace.NavigationBenchmarkCurrentUI()
	if benchmark != nil {
		benchmark.Event("semantic_entry_lookup.begin", "go.ui",
			"entryId", semantic.String(action["entryId"]), "entries", len(fp.Entries))
		defer func() {
			benchmark.Event("semantic_entry_lookup.end", "go.ui",
				"entryId", semantic.String(action["entryId"]), "entries", len(fp.Entries),
				"index", idx, "found", ok, "catalogRevision", fp.catalogRevision)
		}()
	}
	entryID := semantic.String(action["entryId"])
	rawRevision, hasRevision := action["catalogRevision"]
	if semantic.PanelCatalogRowsIsEnabled() && entryID != "" && hasRevision &&
		semantic.Int64(rawRevision) == fp.catalogRevision {
		if rawIndex, present := action["index"]; present {
			candidate := semantic.Int(rawIndex)
			if candidate >= 0 && candidate < len(fp.Entries) &&
				fp.Entries[candidate] != nil {
				sourceKind, _ := fp.semanticSourceInfo()
				actualID, _ := fp.semanticEntryMetadata(
					fp.Entries[candidate], sourceKind)
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
		len(static.entries) == len(fp.Entries) &&
		semantic.Int64(rawRevision) == fp.catalogRevision {
		if candidate, present := static.entryIndexByID[entryID]; present &&
			candidate >= 0 && candidate < len(fp.Entries) {
			sourceKind, _ := fp.semanticSourceInfo()
			actualID, _ := fp.semanticEntryMetadata(fp.Entries[candidate], sourceKind)
			if actualID == entryID {
				return candidate, true
			}
		}
	}

	fp.updateSemanticRevisions()
	if hasRevision && semantic.Int64(rawRevision) != fp.catalogRevision {
		// A useful provisional window can be replaced by the authoritative
		// catalog while an Enter action is already in transit. Opening remains
		// safe when both the row position and its path-derived stable identity
		// still match the current catalog. Keep every other revisioned action
		// strict: stale cursor, pointer, and selection intents must never move to
		// a different row after a replacement.
		if semantic.String(action["action"]) == "panel.open" && entryID != "" {
			if rawIndex, present := action["index"]; present {
				candidate := semantic.Int(rawIndex)
				if candidate >= 0 && candidate < len(fp.Entries) &&
					fp.Entries[candidate] != nil {
					sourceKind, _ := fp.semanticSourceInfo()
					actualID, _ := fp.semanticEntryMetadata(
						fp.Entries[candidate], sourceKind)
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
			len(static.entries) == len(fp.Entries) {
			if candidate, present := static.entryIndexByID[entryID]; present {
				return candidate, true
			}
		}
		sourceKind, _ := fp.semanticSourceInfo()
		for idx, entry := range fp.Entries {
			id, _ := fp.semanticEntryMetadata(entry, sourceKind)
			if id == entryID {
				return idx, true
			}
		}
		// Dropping an action is invisible to the user, so leave a trail: a
		// client whose intent is silently discarded here simply appears to
		// stop responding to clicks.
		vtui.DebugLog("SEMANTIC: dropped action, entryId %q not in %q (%d entries, catalogRevision %d)",
			entryID, fp.Vfs.GetPath(), len(fp.Entries), fp.catalogRevision)
		return 0, false
	}

	if _, ok := action["index"]; !ok {
		return 0, false
	}
	// Preserve the v2 index action behavior: cursor/open clamp through
	// SetCursorIndex and selection toggles outside the range are harmless.
	return semantic.Int(action["index"]), true
}

func (fp *FileSystemPanel) applySemanticSelection(action map[string]any) bool {
	fp.updateSemanticRevisions()
	if rawRevision, ok := action["catalogRevision"]; ok && semantic.Int64(rawRevision) != fp.catalogRevision {
		return false
	}
	if rawRevision, ok := action["selectionRevision"]; ok && semantic.Int64(rawRevision) != fp.selectionRevision {
		return false
	}

	entryIDs := semantic.StringSlice(action["entryIds"])
	if entryID := semantic.String(action["entryId"]); entryID != "" {
		entryIDs = append(entryIDs, entryID)
	}
	wanted := make(map[int]struct{}, len(entryIDs))
	sourceKind, _ := fp.semanticSourceInfo()
	byID := make(map[string]int, len(fp.Entries))
	for idx, entry := range fp.Entries {
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
	for _, idx := range semantic.SemanticIntSlice(action["indices"]) {
		if idx < 0 || idx >= len(fp.Entries) {
			return false
		}
		wanted[idx] = struct{}{}
	}

	selectionChanges := make(map[int]bool)
	if rawChanges, present := action["changes"]; present {
		changes, ok := semantic.SemanticMapSlice(rawChanges)
		if !ok {
			return false
		}
		for _, change := range changes {
			id := semantic.String(change["entryId"])
			selected, hasSelected := change["selected"]
			idx, found := byID[id]
			if id == "" || !hasSelected || !found {
				return false
			}
			selectionChanges[idx] = semantic.Bool(selected)
		}
	}

	cursorIndex := -1
	hasCursor := false
	if cursorID := semantic.String(action["cursorEntryId"]); cursorID != "" {
		var found bool
		cursorIndex, found = byID[cursorID]
		if !found {
			return false
		}
		hasCursor = true
	} else if rawCursorIndex, present := action["cursorIndex"]; present {
		cursorIndex = semantic.Int(rawCursorIndex)
		if cursorIndex < 0 || cursorIndex >= len(fp.Entries) {
			return false
		}
		hasCursor = true
	}

	mode := strings.ToLower(semantic.String(action["mode"]))
	if mode == "" && len(selectionChanges) > 0 {
		mode = "set"
	} else if mode == "" {
		mode = "replace"
	}
	switch mode {
	case "replace":
		for idx := range fp.Entries {
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
	provider, ok := fp.Vfs.(vfs.LocalPathProvider)
	if ok {
		if _, err := provider.LocalPath(fp.Vfs.GetPath()); err == nil {
			return "local", true
		}
	}
	return "vfs", fp.Vfs != nil
}

func (fp *FileSystemPanel) semanticEntryMetadata(entry *FileEntry, sourceKind string) (entryID, logicalPath string) {
	logicalPath = fp.Vfs.Join(fp.Vfs.GetPath(), entry.Name)
	// Stable semantic identity must not perform LocalPath resolution.  Besides
	// being potentially expensive for remote providers, local materialization
	// is deferred metadata and must not advance the base catalog revision.
	identity := sourceKind + "\x00" + fmt.Sprintf("%T", fp.Vfs) + "\x00" + logicalPath
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("entry-%x", sum[:16]), logicalPath
}

func semanticImageExtensions() map[string]struct{} {
	// Snapshotting the decoder registry is extension-only: it
	// never opens a file, invokes a decoder, or probes a plugin.  This gives Qt
	// an explicit answer in the first catalog and prevents synchronous format
	// discovery while delegates are being created.
	extensions := make(map[string]struct{})
	for _, decoder := range media.AllImageDecoders() {
		for _, extension := range decoder.Extensions {
			extension = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
			if extension != "" {
				extensions[extension] = struct{}{}
			}
		}
	}
	return extensions
}

func semanticEntryIsImage(entry *FileEntry, extensions map[string]struct{}) bool {
	if entry == nil || entry.IsDir || entry.Name == ".." {
		return false
	}
	_, ok := extensions[media.ImageExtension(entry.Name)]
	return ok
}

func semanticHighlighterRevision() int64 {
	if theme.GlobalFileHighlighter == nil {
		return 0
	}
	return theme.GlobalFileHighlighter.Revision
}

func semanticHighlighterTimeDependencies() (accessed, created bool) {
	if theme.GlobalFileHighlighter == nil {
		return false, false
	}
	for _, rule := range theme.GlobalFileHighlighter.Rules {
		usesDate := !rule.DateAfter.IsZero() || rule.DateAfterDur > 0 ||
			!rule.DateBefore.IsZero() || rule.DateBeforeDur > 0
		if !usesDate {
			continue
		}
		switch rule.DateType {
		case theme.DateAccessed:
			accessed = true
		case theme.DateCreated:
			created = true
		}
	}
	return accessed, created
}

func (fp *FileSystemPanel) semanticFingerprintsForEntries(entries []*FileEntry) (uint64, uint64, uint64) {
	catalog := fnv.New64a()
	metadata := fnv.New64a()
	selection := fnv.New64a()
	selectedEntryIDs := make([]string, 0)
	sourceKind, previewCapable := fp.semanticSourceInfo()
	imageExtensions := semanticImageExtensions()
	highlightUsesATime, highlightUsesCTime := semanticHighlighterTimeDependencies()
	semantic.WriteSemanticFingerprintString(catalog, sourceKind)
	semantic.WriteSemanticFingerprintString(catalog, fp.Vfs.GetPath())
	semantic.WriteSemanticFingerprintString(metadata, sourceKind)
	semantic.WriteSemanticFingerprintString(metadata, fp.Vfs.GetPath())
	semantic.SemanticWriteUint64(metadata, uint64(semanticHighlighterRevision()))
	// This presentation option changes the normalized display fields consumed
	// by unified Columns/Details without changing any VFS entry. Treat it as a
	// catalog change so persistent Gallery sessions receive the new appearance
	// instead of retaining their previous name split indefinitely.
	if config.App.SeparateFileExtensions {
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
		semantic.WriteSemanticFingerprintString(catalog, entryID)
		semantic.WriteSemanticFingerprintString(catalog, logicalPath)
		semantic.WriteSemanticFingerprintString(catalog, entry.Name)
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

		semantic.WriteSemanticFingerprintString(metadata, entryID)
		semantic.WriteSemanticFingerprintString(metadata, entry.Mode)
		// Provider revisions describe file content, not row identity/layout.
		// Keep them out of CatalogRevision while still invalidating deferred
		// metadata and broker-backed derived artifacts when bytes change.
		semantic.WriteSemanticFingerprintString(metadata, entry.Revision)
		semantic.SemanticWriteUint64(metadata, uint64(entry.Size))
		semantic.SemanticWriteUint64(metadata, uint64(semantic.SemanticMTimeNanos(entry.MTime)))
		if highlightUsesATime {
			semantic.SemanticWriteUint64(metadata, uint64(semantic.SemanticMTimeNanos(entry.ATime)))
		}
		if highlightUsesCTime {
			semantic.SemanticWriteUint64(metadata, uint64(semantic.SemanticMTimeNanos(entry.CTime)))
		}
		// Highlight rules can derive read-only/system/archive state from these
		// platform mode fields even though the raw values are not serialized.
		semantic.SemanticWriteUint64(metadata, uint64(entry.UnixMode))
		semantic.SemanticWriteUint64(metadata, uint64(entry.WinAttrs))
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
		semantic.WriteSemanticFingerprintString(selection, entryID)
	}
	return catalog.Sum64(), metadata.Sum64(), selection.Sum64()
}

func (fp *FileSystemPanel) semanticFingerprints() (uint64, uint64, uint64) {
	return fp.semanticFingerprintsForEntries(fp.Entries)
}

func (fp *FileSystemPanel) updateSemanticRevisions() {
	if semantic.PanelCatalogRowsIsEnabled() {
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
	if catalogChanged || (!semantic.PanelCatalogMetadataIsEnabled() && metadataChanged) {
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

func (fp *FileSystemPanel) SemanticSelectionPatch(baseRevision int64) (extui.PanelPatch, bool) {
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
	selected := make([]string, 0, len(fp.SelectedItems))
	if cache != nil && cache.catalogRevision == fp.catalogRevision && len(cache.entries) == len(fp.Entries) {
		for index, entry := range fp.Entries {
			if entry.Selected {
				selected = append(selected, cache.entries[index].EntryID)
			}
		}
	} else {
		sourceKind, _ := fp.semanticSourceInfo()
		for _, entry := range fp.Entries {
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

func (fp *FileSystemPanel) AcknowledgeSemanticSelection(revision int64) {
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
	mediaBroker           panelMediaRegistry
	mediaSourceEpoch      int64
}

func emptySemanticSelectionFingerprint() uint64 {
	return fnv.New64a().Sum64()
}

func (fp *FileSystemPanel) semanticStaticPanelData(sourceKind string) *semanticPanelStaticCache {
	mediaBroker := currentPanelMediaRegistry()
	metadataDeferred := semantic.PanelCatalogMetadataIsEnabled()
	benchmark := navtrace.NavigationBenchmarkCurrentUI()
	cache := fp.semanticStaticCache
	if cache != nil &&
		cache.catalogRevision == fp.catalogRevision &&
		cache.separateFileExtension == config.App.SeparateFileExtensions &&
		cache.highlighterRevision == semanticHighlighterRevision() &&
		cache.mediaBroker == mediaBroker &&
		cache.mediaSourceEpoch == fp.mediaSourceEpoch {
		return cache
	}

	imageExtensions := semanticImageExtensions()
	cache = &semanticPanelStaticCache{
		catalogRevision:       fp.catalogRevision,
		separateFileExtension: config.App.SeparateFileExtensions,
		highlighterRevision:   semanticHighlighterRevision(),
		highlightStyles:       make(map[string]extui.HighlightStyleModel),
		entries:               make([]extui.FileEntryModel, 0, len(fp.Entries)),
		entryIndexByID:        make(map[string]int, len(fp.Entries)),
		mediaBroker:           mediaBroker,
		mediaSourceEpoch:      fp.mediaSourceEpoch,
	}
	panelID := vtui.SemanticID(fp)
	resourceIDs := make([]string, 0, len(fp.Entries))
	caps := fp.Vfs.GetCapabilities()
	rowsStartedNs := int64(0)
	if benchmark != nil {
		rowsStartedNs = navtrace.NavigationBenchmarkMonotonicNs()
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
	eagerStyle := !metadataDeferred || len(fp.Entries) <= eagerDeferredStyleRows
	for i, entry := range fp.Entries {
		entryID, logicalPath := fp.semanticEntryMetadata(entry, sourceKind)
		cache.entryIndexByID[entryID] = i
		isImage := semanticEntryIsImage(entry, imageExtensions)
		if isImage {
			imageCount++
		}
		displayName := entry.visibleName()
		displayBaseName := displayName
		displayExtension := ""
		if config.App.SeparateFileExtensions && !entry.NoExtension && !entry.IsDir && entry.Name != ".." {
			if base, extension := splitFileExtension(displayName); extension != "" {
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
				styleStartedNs = navtrace.NavigationBenchmarkMonotonicNs()
			}
			highlightStyleID, highlightStyle = theme.GlobalFileHighlighter.SemanticStyle(
				&entry.VFSItem, false)
			if benchmark != nil {
				styleDurationNs += navtrace.NavigationBenchmarkMonotonicNs() - styleStartedNs
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
			sourceStartedNs = navtrace.NavigationBenchmarkMonotonicNs()
		}
		if needsSourceDescriptor {
			entryPath := logicalPath
			storage := plughost.MediaStorageClass(caps, "")
			var versionStrength string
			version, versionStrength = plughost.MediaSourceVersion(fp.Vfs, entry.VFSItem, storage == vfs.StorageClassLocal, fp.catalogRevision, fp.mediaSourceEpoch)
			source := extui.ImageSourceModel{
				SourceKey:       plughost.MediaSourceKey(fp.Vfs, entryPath),
				Version:         version,
				VersionStrength: versionStrength,
				Size:            entry.Size,
				SizeKnown:       entry.SizeKnown || entry.Size != 0,
				AccessProfile:   caps.ReadAccess.String(),
				StorageClass:    storage.String(),
			}
			if mediaBroker != nil && !entry.IsDir && entry.Name != ".." {
				descriptor := mediaBroker.Register(plughost.MediaSourceRegistration{
					PanelID: panelID, CatalogVersion: fp.catalogRevision, SourceEpoch: fp.mediaSourceEpoch, FS: fp.Vfs,
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
			sourceDurationNs += navtrace.NavigationBenchmarkMonotonicNs() - sourceStartedNs
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
		rowsFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
		benchmark.EventAt("model.semantic_static.rows", "go.ui", rowsFinishedNs,
			"entries", len(fp.Entries), "images", imageCount,
			"styleDurationNs", styleDurationNs, "sourceDurationNs", sourceDurationNs,
			"durationNs", rowsFinishedNs-rowsStartedNs)
	}
	if mediaBroker != nil {
		commitStartedNs := int64(0)
		if benchmark != nil {
			commitStartedNs = navtrace.NavigationBenchmarkMonotonicNs()
		}
		mediaBroker.CommitPanel(panelID, fp.catalogRevision, resourceIDs)
		if benchmark != nil {
			commitFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
			benchmark.EventAt("model.semantic_static.commit", "go.ui", commitFinishedNs,
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
	highlighter       *theme.FileHighlighter
	entries           []semanticPanelMetadataEntry
	totalSize         int64
}

var semanticPanelMetadataSnapshots sync.Map

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

func cloneSemanticFileHighlighter(source *theme.FileHighlighter) *theme.FileHighlighter {
	if source == nil {
		return nil
	}
	clone := &theme.FileHighlighter{Revision: source.Revision, Rules: make([]theme.HighlightRule, len(source.Rules))}
	copy(clone.Rules, source.Rules)
	for i := range clone.Rules {
		clone.Rules[i].Masks = append([]string(nil), source.Rules[i].Masks...)
	}
	return clone
}

func (fp *FileSystemPanel) publishSemanticMetadataSnapshot(panelID string, baseEntries []extui.FileEntryModel) {
	path := fp.Vfs.GetPath()
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
		highlighter:       cloneSemanticFileHighlighter(theme.GlobalFileHighlighter),
		entries:           make([]semanticPanelMetadataEntry, 0, len(fp.Entries)),
	}
	if provider, ok := fp.Vfs.(vfs.LocalPathProvider); ok {
		snapshot.provider = provider
	}
	for index, entry := range fp.Entries {
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

func (fp *FileSystemPanel) CommitSemanticMetadataMutation() {
	if fp == nil || !semantic.PanelCatalogMetadataIsEnabled() {
		return
	}
	if semantic.PanelCatalogRowsIsEnabled() {
		sourceKind, _ := fp.semanticSourceInfo()
		fp.ensureSemanticPagedRevisions(sourceKind)
		fp.metadataRevision++
		return
	}
	fp.updateSemanticRevisions()
	static := fp.semanticStaticCache
	if static == nil || static.catalogRevision != fp.catalogRevision ||
		len(static.entries) != len(fp.Entries) {
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
		entry := FileEntry{VFSItem: source.item, SizeCalculated: source.sizeCalculated}
		mtimeNanos := semantic.SemanticMTimeNanos(source.item.MTime)
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
	// Completed folders below this size are sent as a dense catalog. Masonry
	// needs every image row in one model so ZoinGallery can use the decoded
	// natural dimensions instead of its uniform sparse-grid geometry. Larger
	// folders keep the sparse protocol and its bounded memory cost.
	completePanelCatalogRowsLimit = 512
	// Fast Find is transient viewport decoration. Keep its exported match map
	// bounded to painted/buffered rows instead of serializing one entry for an
	// entire large directory on every keystroke.
	semanticFastFindRowsLimit        = maxPanelCatalogRowsLimit
	semanticFastFindDetailsRowsLimit = 128
)

var semanticLivePanels sync.Map

type semanticPagedEntrySource struct {
	entries []*FileEntry
	up      *FileEntry
}

func (source semanticPagedEntrySource) count() int {
	count := len(source.entries)
	if source.up != nil {
		count++
	}
	return count
}

func (source semanticPagedEntrySource) entryAt(index int) *FileEntry {
	if index < 0 {
		return nil
	}
	if source.up != nil {
		if index == 0 {
			return source.up
		}
		index--
	}
	if index >= len(source.entries) {
		return nil
	}
	return source.entries[index]
}

func (fp *FileSystemPanel) resetSemanticPendingSource(generation uint64) {
	if fp == nil {
		return
	}
	fp.semanticPendingMu.Lock()
	fp.semanticPendingLoadGeneration = generation
	fp.semanticPendingEntries = nil
	fp.semanticPendingUpEntry = nil
	fp.semanticPendingMu.Unlock()
}

func (fp *FileSystemPanel) setSemanticPendingSource(
	generation uint64, entries []*FileEntry, includeUp bool,
) {
	if fp == nil {
		return
	}
	var up *FileEntry
	if includeUp {
		up = &FileEntry{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}
	}
	fp.semanticPendingMu.Lock()
	if fp.semanticPendingLoadGeneration == generation {
		fp.semanticPendingEntries = entries
		fp.semanticPendingUpEntry = up
	}
	fp.semanticPendingMu.Unlock()
}

func (fp *FileSystemPanel) clearSemanticPendingSource(generation uint64) {
	if fp == nil {
		return
	}
	fp.semanticPendingMu.Lock()
	if fp.semanticPendingLoadGeneration == generation {
		fp.semanticPendingEntries = nil
		fp.semanticPendingUpEntry = nil
	}
	fp.semanticPendingMu.Unlock()
}

func (fp *FileSystemPanel) semanticPagedEntrySource() semanticPagedEntrySource {
	if fp == nil {
		return semanticPagedEntrySource{}
	}
	fp.semanticPendingMu.RLock()
	pendingGeneration := fp.semanticPendingLoadGeneration
	pendingEntries := fp.semanticPendingEntries
	pendingUpEntry := fp.semanticPendingUpEntry
	fp.semanticPendingMu.RUnlock()
	if pendingGeneration == fp.loadGeneration &&
		(pendingEntries != nil || pendingUpEntry != nil) {
		return semanticPagedEntrySource{
			entries: pendingEntries,
			up:      pendingUpEntry,
		}
	}
	return semanticPagedEntrySource{entries: fp.Entries}
}

func (fp *FileSystemPanel) markSemanticCatalogMutation() {
	if fp == nil {
		return
	}
	fp.semanticCatalogGeneration++
	fp.semanticStaticCache = nil
}

func (fp *FileSystemPanel) semanticPagedModelSignature(sourceKind string) string {
	if fp == nil || fp.Vfs == nil {
		return ""
	}
	return fmt.Sprintf("%s\x00%T\x00%s\x00%d\x00%t\x00%t\x00%d",
		sourceKind, fp.Vfs, fp.Vfs.GetPath(), fp.SortMode, fp.SortReverse,
		config.App.SeparateFileExtensions, fp.mediaSourceEpoch)
}

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

func (fp *FileSystemPanel) semanticCatalogTotalCount() int {
	if fp == nil {
		return 0
	}
	materializedCount := len(fp.Entries)
	if pending := fp.semanticPagedEntrySource().count(); pending > materializedCount {
		materializedCount = pending
	}
	if fp.catalogLogicalCount > materializedCount {
		return fp.catalogLogicalCount
	}
	return materializedCount
}

func (fp *FileSystemPanel) semanticCatalogCanBeDense() bool {
	if fp == nil || fp.Vfs == nil || fp.IsLoading || fp.catalogProvisional ||
		fp.catalogLogicalCount != 0 || len(fp.Entries) > completePanelCatalogRowsLimit {
		return false
	}
	source := fp.semanticPagedEntrySource()
	return source.count() == len(fp.Entries)
}

func (fp *FileSystemPanel) semanticPageStartsBeyondMaterializedSource(
	offset, sourceCount int,
) bool {
	if fp == nil || offset < 0 || sourceCount < 0 {
		return true
	}
	return offset > sourceCount ||
		(offset < fp.semanticCatalogTotalCount() && offset >= sourceCount)
}

func (fp *FileSystemPanel) semanticFastFindRange() (int, int, bool) {
	if fp == nil || !fp.FastFindMode || fp.FastFindStr == "" || len(fp.Entries) == 0 {
		return 0, 0, false
	}
	first, end := semanticFastFindWindowRange(
		len(fp.Entries), fp.GetCursorIndex(), fp.semanticFastFindWindowLimit())
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
	if fp == nil || !fp.FastFindMode || fp.FastFindStr == "" || entryIDAt == nil {
		return nil
	}
	fp.ensureFastFindMatchCache()
	if fp.fastFindAnyMatchKnown && !fp.fastFindAnyMatch {
		return nil
	}
	first = max(0, first)
	end = min(len(fp.Entries), end)
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
	if fp == nil || fp.Vfs == nil || offset < 0 {
		return nil, nil, false
	}
	source := fp.semanticPagedEntrySource()
	if fp.semanticPageStartsBeyondMaterializedSource(offset, source.count()) {
		return nil, nil, false
	}
	if limit <= 0 {
		limit = initialPanelCatalogRowsLimit
	}
	if limit > maxPanelCatalogRowsLimit && !fp.semanticCatalogCanBeDense() {
		limit = maxPanelCatalogRowsLimit
	}
	end := offset + limit
	if end > source.count() {
		end = source.count()
	}
	sourceKind, _ := fp.semanticSourceInfo()
	imageExtensions := semanticImageExtensions()
	styles := make(map[string]extui.HighlightStyleModel)
	rows := make([]extui.FileEntryModel, 0, end-offset)
	mediaBroker := currentPanelMediaRegistry()
	panelID := vtui.SemanticID(fp)
	if fp.semanticPagedResourceRevision != fp.catalogRevision {
		fp.semanticPagedResourceRevision = fp.catalogRevision
		fp.semanticPagedResourceIDs = make(map[string]struct{})
	}
	caps := fp.Vfs.GetCapabilities()
	for index := offset; index < end; index++ {
		entry := source.entryAt(index)
		if entry == nil {
			return nil, nil, false
		}
		entryID, logicalPath := fp.semanticEntryMetadata(entry, sourceKind)
		displayBaseName := entry.Name
		displayExtension := ""
		if config.App.SeparateFileExtensions && !entry.NoExtension &&
			!entry.IsDir && entry.Name != ".." {
			if base, extension := splitFileExtension(entry.Name); extension != "" {
				displayBaseName, displayExtension = base, extension
			}
		}
		highlightStyleID, highlightStyle := theme.GlobalFileHighlighter.SemanticStyle(
			&entry.VFSItem, false)
		if highlightStyleID != "" {
			styles[highlightStyleID] = highlightStyle
		}
		isImage := semanticEntryIsImage(entry, imageExtensions)
		version := ""
		var sourceModel *extui.ImageSourceModel
		if isImage {
			storage := plughost.MediaStorageClass(caps, "")
			var versionStrength string
			version, versionStrength = plughost.MediaSourceVersion(
				fp.Vfs, entry.VFSItem, storage == vfs.StorageClassLocal,
				fp.catalogRevision, fp.mediaSourceEpoch)
			source := extui.ImageSourceModel{
				SourceKey:       plughost.MediaSourceKey(fp.Vfs, logicalPath),
				Version:         version,
				VersionStrength: versionStrength,
				Size:            entry.Size,
				SizeKnown:       entry.SizeKnown || entry.Size != 0,
				AccessProfile:   caps.ReadAccess.String(),
				StorageClass:    storage.String(),
			}
			if mediaBroker != nil {
				descriptor := mediaBroker.Register(plughost.MediaSourceRegistration{
					PanelID: panelID, CatalogVersion: fp.catalogRevision,
					SourceEpoch: fp.mediaSourceEpoch, FS: fp.Vfs,
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
	if !ok || fp == nil || fp.Vfs == nil || fp.Vfs.GetPath() != path ||
		fp.catalogRevision != catalogRevision || offset < 0 {
		return nil, false
	}
	source := fp.semanticPagedEntrySource()
	if fp.semanticPageStartsBeyondMaterializedSource(offset, source.count()) {
		return nil, false
	}
	rows, styles, ok := fp.semanticPagedRows(offset, limit)
	if !ok || len(rows) == 0 && offset != source.count() {
		return nil, false
	}
	return extui.PanelCatalogRowsModel{
		PanelID: panelID, Path: path, CatalogRevision: catalogRevision,
		Offset: offset, Limit: len(rows), Total: fp.semanticCatalogTotalCount(),
		Entries: rows, HighlightStyles: styles,
	}.ToMap(), true
}

func LivePanelCatalogRowsRetryable(panelID, path string,
	catalogRevision int64) bool {
	loaded, ok := semanticLivePanels.Load(panelID)
	if !ok {
		return false
	}
	fp, ok := loaded.(*FileSystemPanel)
	return ok && fp != nil && fp.Vfs != nil && fp.Vfs.GetPath() == path &&
		fp.catalogRevision == catalogRevision && fp.IsLoading &&
		fp.catalogLogicalCount > len(fp.Entries)
}

func BuildLivePanelCatalogMetadataChunk(panelID, path string,
	catalogRevision, metadataRevision int64, offset, limit int,
) (map[string]any, bool) {
	loaded, ok := semanticLivePanels.Load(panelID)
	if !ok {
		return nil, false
	}
	fp, ok := loaded.(*FileSystemPanel)
	if !ok || fp == nil || fp.Vfs == nil || fp.Vfs.GetPath() != path ||
		fp.catalogRevision != catalogRevision || fp.metadataRevision != metadataRevision ||
		offset < 0 {
		return nil, false
	}
	source := fp.semanticPagedEntrySource()
	if fp.semanticPageStartsBeyondMaterializedSource(offset, source.count()) {
		return nil, false
	}
	if limit <= 0 {
		limit = defaultPanelCatalogMetadataChunkLimit
	}
	if limit > maxPanelCatalogMetadataChunkLimit {
		limit = maxPanelCatalogMetadataChunkLimit
	}
	end := offset + limit
	if end > source.count() {
		end = source.count()
	}
	sourceKind, _ := fp.semanticSourceInfo()
	provider, _ := fp.Vfs.(vfs.LocalPathProvider)
	chunk := extui.PanelCatalogMetadataModel{
		PanelID: panelID, Path: path, CatalogRevision: catalogRevision,
		MetadataRevision:  metadataRevision,
		HighlightRevision: semanticHighlighterRevision(),
		Offset:            offset, Limit: end - offset, Total: fp.semanticCatalogTotalCount(),
		Final:           end == fp.semanticCatalogTotalCount(),
		Entries:         make([]extui.FileEntryMetadataModel, 0, end-offset),
		HighlightStyles: make(map[string]extui.HighlightStyleModel),
	}
	for index := offset; index < end; index++ {
		entry := source.entryAt(index)
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
		highlightStyleID, highlightStyle := theme.GlobalFileHighlighter.SemanticStyle(
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
			MTimeNanos: semantic.SemanticMTimeNanos(entry.MTime), Mode: entry.Mode,
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
	totalCount := fp.semanticCatalogTotalCount()
	denseCatalog := fp.semanticCatalogCanBeDense()
	limit := initialPanelCatalogRowsLimit
	if denseCatalog {
		limit = totalCount
	}
	offset, end := semanticPanelCatalogRange(totalCount, cursor, limit)
	entries, highlightStyles, _ := fp.semanticPagedRows(offset, end-offset)
	cursorEntryID := ""
	if cursor >= 0 && cursor < len(fp.Entries) {
		cursorEntryID, _ = fp.semanticEntryMetadata(fp.Entries[cursor], sourceKind)
	}
	fastFindMatches := fp.semanticFastFindMatches(
		offset, end, func(index int) string {
			return entries[index-offset].EntryID
		})
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.GalleryLayoutRevision
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	semanticTitle := fp.semanticTitle
	if semanticTitle == "" {
		semanticTitle = fp.currentTitle
	}
	return extui.PanelModel{
		PathIcon: semanticPanelIcon(fp.Vfs),
		ID:       panelID, Side: side, Active: active, Path: fp.Vfs.GetPath(),
		Title: semanticTitle, ShowFileInfo: config.App.ShowPanelFileInfo,
		GalleryLayoutMode:     string(galleryLayoutMode),
		GalleryColumnCount:    fp.effectiveGalleryColumnCount(),
		GalleryDensity:        fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:      fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision: galleryLayoutRevision,
		DropAllowed:           VfsAcceptsDrop(fp.Vfs),
		SourceKind:            sourceKind, PreviewCapable: previewCapable,
		CatalogRevision:     fp.catalogRevision,
		SelectionRevision:   fp.selectionRevision,
		MetadataDeferred:    true,
		MetadataRevision:    fp.metadataRevision,
		CatalogRowsDeferred: !denseCatalog,
		CatalogDelta:        fp.semanticCatalogDelta(),
		HighlightRevision:   semanticHighlighterRevision(),
		HighlightStyles:     highlightStyles,
		CursorEntryID:       cursorEntryID,
		SortMode:            sortModeName(fp.SortMode), SortReverse: fp.SortReverse,
		SeparateFileExtensions: config.App.SeparateFileExtensions,
		Cursor:                 cursor, Loading: fp.semanticLoading(),
		CatalogProvisional: fp.catalogProvisional,
		FastFind:           fp.FastFindMode, FastFindText: fp.FastFindStr,
		FastFindMatchColor: func() string {
			if fp.FastFindMode && fp.FastFindStr != "" {
				return semantic.SemanticAttrColor(vtui.Palette[theme.ColPanelHighlightText], true)
			}
			return ""
		}(),
		FastFindMatches: fastFindMatches,
		SelectedCount:   len(fp.SelectedItems), TotalCount: totalCount,
		GalleryColumns: fp.semanticGalleryColumns(), Entries: entries,
	}
}

func (fp *FileSystemPanel) semanticLoading() bool {
	return fp != nil && fp.IsLoading && !fp.catalogInteractive
}

func (fp *FileSystemPanel) SemanticPanelModel(ctx *vtui.SemanticContext, side int, active bool) (model extui.PanelModel) {
	defer func() { fp.enrichNativePanelStatus(&model) }()

	if semantic.PanelCatalogRowsIsEnabled() {
		return fp.semanticPagedPanelModel(ctx, side, active)
	}
	benchmark := navtrace.NavigationBenchmarkCurrentUI()
	metadataDeferred := semantic.PanelCatalogMetadataIsEnabled()
	sourceKind, previewCapable := fp.semanticSourceInfo()
	revisionsStartedNs := int64(0)
	if benchmark != nil {
		revisionsStartedNs = navtrace.NavigationBenchmarkMonotonicNs()
	}
	fp.updateSemanticRevisions()
	if benchmark != nil {
		revisionsFinishedNs := navtrace.NavigationBenchmarkMonotonicNs()
		benchmark.EventAt("model.semantic_revisions", "go.ui", revisionsFinishedNs,
			"durationNs", revisionsFinishedNs-revisionsStartedNs)
	}
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.GalleryLayoutRevision
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
		fastFindMatchColor = semantic.SemanticAttrColor(
			vtui.Palette[theme.ColPanelHighlightText], true)
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
		localPathProvider, _ = fp.Vfs.(vfs.LocalPathProvider)
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
	for i, entry := range fp.Entries {
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
		entries[i].MTimeNanos = semantic.SemanticMTimeNanos(entry.MTime)
		entries[i].Version = fmt.Sprintf("%d:%d", entries[i].MTimeNanos, entry.Size)
		entries[i].Mode = entry.Mode
		highlightStyleID, highlightStyle := theme.GlobalFileHighlighter.SemanticStyle(&entry.VFSItem, true)
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
	// Publishing a catalog keeps the drag/request owner alive even when
	// deferred metadata is disabled and its separate snapshot was released.
	semanticLivePanels.Store(panelID, fp)
	return extui.PanelModel{
		PathIcon:               semanticPanelIcon(fp.Vfs),
		ID:                     panelID,
		Side:                   side,
		Active:                 active,
		Path:                   fp.Vfs.GetPath(),
		Title:                  semanticTitle,
		ShowFileInfo:           config.App.ShowPanelFileInfo,
		GalleryLayoutMode:      string(galleryLayoutMode),
		GalleryColumnCount:     fp.effectiveGalleryColumnCount(),
		GalleryDensity:         fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:       fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision:  galleryLayoutRevision,
		DropAllowed:            VfsAcceptsDrop(fp.Vfs),
		SourceKind:             sourceKind,
		PreviewCapable:         previewCapable,
		CatalogRevision:        fp.catalogRevision,
		SelectionRevision:      fp.selectionRevision,
		MetadataDeferred:       metadataDeferred,
		MetadataRevision:       fp.metadataRevision,
		HighlightRevision:      highlightRevision,
		HighlightStyles:        highlightStyles,
		CursorEntryID:          cursorEntryID,
		SortMode:               sortModeName(fp.SortMode),
		SortReverse:            fp.SortReverse,
		SeparateFileExtensions: config.App.SeparateFileExtensions,
		Cursor:                 fp.GetCursorIndex(),
		Loading:                fp.semanticLoading(),
		CatalogProvisional:     fp.catalogProvisional,
		FastFind:               fp.FastFindMode,
		FastFindText:           fp.FastFindStr,
		FastFindMatchColor:     fastFindMatchColor,
		FastFindMatches:        fastFindMatches,
		SelectedCount:          selectedCount,
		SelectedSize:           selectedSize,
		TotalCount:             len(fp.Entries),
		TotalSize:              totalSize,
		GalleryColumns:         fp.semanticGalleryColumns(),
		Entries:                entries,
	}
}

func (fp *FileSystemPanel) semanticPagedPanelHeaderModel(
	ctx *vtui.SemanticContext, side int, active bool,
) (extui.PanelModel, bool) {
	if fp == nil || fp.Vfs == nil {
		return extui.PanelModel{}, false
	}
	sourceKind, previewCapable := fp.semanticSourceInfo()
	fp.ensureSemanticPagedRevisions(sourceKind)
	panelID := vtui.SemanticID(fp)
	semanticLivePanels.Store(panelID, fp)
	cursor := fp.GetCursorIndex()
	cursorEntryID := ""
	if cursor >= 0 && cursor < len(fp.Entries) && fp.Entries[cursor] != nil {
		cursorEntryID, _ = fp.semanticEntryMetadata(fp.Entries[cursor], sourceKind)
	}
	var fastFindMatches map[string]extui.FastFindMatchModel
	fastFindMatchColor := ""
	if first, end, active := fp.semanticFastFindRange(); active {
		fastFindMatchColor = semantic.SemanticAttrColor(
			vtui.Palette[theme.ColPanelHighlightText], true)
		fastFindMatches = fp.semanticFastFindMatches(
			first, end, func(index int) string {
				entry := fp.Entries[index]
				if entry == nil {
					return ""
				}
				entryID, _ := fp.semanticEntryMetadata(entry, sourceKind)
				return entryID
			})
	}
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.GalleryLayoutRevision
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	semanticTitle := fp.semanticTitle
	if semanticTitle == "" {
		semanticTitle = fp.currentTitle
	}
	denseCatalog := fp.semanticCatalogCanBeDense()
	return extui.PanelModel{
		PathIcon: semanticPanelIcon(fp.Vfs),
		ID:       panelID, Side: side, Active: active, Path: fp.Vfs.GetPath(),
		Title: semanticTitle, ShowFileInfo: config.App.ShowPanelFileInfo,
		GalleryLayoutMode:     string(galleryLayoutMode),
		GalleryColumnCount:    fp.effectiveGalleryColumnCount(),
		GalleryDensity:        fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:      fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision: galleryLayoutRevision,
		DropAllowed:           VfsAcceptsDrop(fp.Vfs),
		SourceKind:            sourceKind, PreviewCapable: previewCapable,
		CatalogRevision:     fp.catalogRevision,
		SelectionRevision:   fp.selectionRevision,
		MetadataDeferred:    true,
		MetadataRevision:    fp.metadataRevision,
		CatalogRowsDeferred: !denseCatalog,
		HighlightRevision:   semanticHighlighterRevision(),
		CursorEntryID:       cursorEntryID,
		SortMode:            sortModeName(fp.SortMode), SortReverse: fp.SortReverse,
		SeparateFileExtensions: config.App.SeparateFileExtensions,
		Cursor:                 cursor, Loading: fp.semanticLoading(),
		CatalogProvisional: fp.catalogProvisional,
		FastFind:           fp.FastFindMode, FastFindText: fp.FastFindStr,
		FastFindMatchColor: fastFindMatchColor,
		FastFindMatches:    fastFindMatches,
		SelectedCount:      len(fp.SelectedItems), TotalCount: fp.semanticCatalogTotalCount(),
		GalleryColumns: fp.semanticGalleryColumns(),
	}, true
}

func (fp *FileSystemPanel) semanticPanelHeaderModel(ctx *vtui.SemanticContext, side int, active bool) (model extui.PanelModel, valid bool) {
	defer func() {
		if valid {
			fp.enrichNativePanelStatus(&model)
		}
	}()

	if semantic.PanelCatalogRowsIsEnabled() {
		return fp.semanticPagedPanelHeaderModel(ctx, side, active)
	}
	if fp == nil || !semantic.PanelCatalogMetadataIsEnabled() {
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
		len(static.entries) != len(fp.Entries)
	if len(static.entries) != len(fp.Entries) && !provisionalReplacement {
		return extui.PanelModel{}, false
	}
	galleryLayoutMode := fp.effectiveGalleryLayoutMode()
	galleryLayoutRevision := fp.GalleryLayoutRevision
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
		fastFindMatchColor = semantic.SemanticAttrColor(vtui.Palette[theme.ColPanelHighlightText], true)
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
		PathIcon:               semanticPanelIcon(fp.Vfs),
		ID:                     vtui.SemanticID(fp),
		Side:                   side,
		Active:                 active,
		Path:                   fp.Vfs.GetPath(),
		Title:                  semanticTitle,
		ShowFileInfo:           config.App.ShowPanelFileInfo,
		GalleryLayoutMode:      string(galleryLayoutMode),
		GalleryColumnCount:     fp.effectiveGalleryColumnCount(),
		GalleryDensity:         fp.galleryDensity(galleryLayoutMode),
		GalleryDensities:       fp.galleryDensitiesSnapshot(),
		GalleryLayoutRevision:  galleryLayoutRevision,
		DropAllowed:            VfsAcceptsDrop(fp.Vfs),
		SourceKind:             sourceKind,
		PreviewCapable:         previewCapable,
		CatalogRevision:        fp.catalogRevision,
		SelectionRevision:      fp.selectionRevision,
		MetadataDeferred:       true,
		MetadataRevision:       fp.metadataRevision,
		HighlightRevision:      static.highlighterRevision,
		CursorEntryID:          cursorEntryID,
		SortMode:               sortModeName(fp.SortMode),
		SortReverse:            fp.SortReverse,
		SeparateFileExtensions: config.App.SeparateFileExtensions,
		Cursor:                 cursor,
		Loading:                fp.semanticLoading(),
		CatalogProvisional:     fp.catalogProvisional,
		FastFind:               fp.FastFindMode,
		FastFindText:           fp.FastFindStr,
		FastFindMatchColor:     fastFindMatchColor,
		FastFindMatches:        fastFindMatches,
		SelectedCount: func() int {
			if provisionalReplacement {
				return 0
			}
			return len(fp.SelectedItems)
		}(),
		TotalCount:     len(static.entries),
		GalleryColumns: fp.semanticGalleryColumns(),
	}, true
}

func (fp *FileSystemPanel) semanticGalleryColumns() []extui.PanelColumnModel {
	width := fp.X2 - fp.X1 + 1
	nameWidth := width - 14
	if nameWidth < 5 {
		nameWidth = 5
	}
	title := func(base string, mode SortMode) string {
		if fp.SortMode != mode {
			return base
		}
		if fp.SortIsAscending() {
			return base + " ↑"
		}
		return base + " ↓"
	}
	return []extui.PanelColumnModel{
		{
			ID:        "name",
			Role:      "name",
			Index:     0,
			Title:     title(i18n.Msg("Panel.Column.Name"), SortName),
			Width:     nameWidth,
			Alignment: "left",
			SortMode:  sortModeName(SortName),
			Sortable:  true,
		},
		{
			ID:        "size",
			Role:      "size",
			Index:     1,
			Title:     title(i18n.Msg("Panel.Column.Size"), SortSize),
			Width:     panelSizeColumnWidth,
			Alignment: "right",
			SortMode:  sortModeName(SortSize),
			Sortable:  true,
		},
	}
}

func semanticFileSizeValue(entry *FileEntry) int64 {
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

func semanticFileSize(entry *FileEntry) string {
	if entry == nil {
		return ""
	}
	if entry.IsDir {
		if entry.SizeCalculated {
			return fileops.FormatIntWithSpaces(entry.Size)
		}
		if entry.Name == ".." {
			return i18n.Msg("Panel.UpDir")
		}
		return ""
	}
	size := semanticFileSizeValue(entry)
	if size < 0 {
		return ""
	}
	return fileops.FormatIntWithSpaces(size)
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

func semanticPanelIcon(filesystem vfs.VFS) string {
	if provider, ok := filesystem.(vfs.PanelIconProvider); ok {
		if icon := strings.TrimSpace(provider.PanelIcon()); icon != "" {
			return icon
		}
	}
	if filesystem == nil {
		return ""
	}
	if _, ok := filesystem.(*vfs.OSVFS); ok {
		return ""
	}
	return "plug"
}

func (fp *FileSystemPanel) semanticCatalogDelta() extui.M {
	if !semantic.PanelCatalogDeltaEnabled.Load() || fp.catalogRefreshDelta == nil {
		return nil
	}
	base, _ := fp.catalogRefreshDelta["baseCatalogRevision"].(int64)
	if base+1 != fp.catalogRevision {
		return nil
	}
	return fp.catalogRefreshDelta
}
