package main

import (
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func init() {
	vtui.AppSceneAdapter = BuildAppSceneFromLegacy
}

func BuildAppSceneFromLegacy(ctx *vtui.SemanticContext, legacy map[string]any) map[string]any {
	if legacy == nil {
		return nil
	}
	autocompletes := appActiveAutocompleteMenus()
	scene := extui.Scene{
		Width:          semanticInt(legacy["width"]),
		Height:         semanticInt(legacy["height"]),
		ActiveScreen:   semanticInt(legacy["activeScreen"]),
		WorkspaceCount: semanticInt(legacy["workspaceCount"]),
		Legacy:         legacy,
	}
	if scene.Width == 0 && ctx != nil {
		scene.Width = ctx.Width
	}
	if scene.Height == 0 && ctx != nil {
		scene.Height = ctx.Height
	}
	if ctx != nil && scene.ActiveScreen == 0 {
		scene.ActiveScreen = ctx.ActiveScreen
	}

	if menu := appMap(legacy["menuBar"]); menu != nil {
		m := appMenuFromLegacy(menu, "menuBar")
		scene.MenuBar = &m
	}
	if keyBar := appMap(legacy["keyBar"]); keyBar != nil {
		k := appKeyBarFromLegacy(keyBar)
		scene.KeyBar = &k
	}
	if toast := appMap(legacy["toast"]); toast != nil {
		scene.Toast = &extui.ToastModel{Message: semanticString(toast["message"])}
	}

	for _, frame := range appMapSlice(legacy["frames"]) {
		if autocompletes.isLegacyFrame(frame) {
			continue
		}
		switch semanticString(frame["kind"]) {
		case "panels", "shell":
			shell := appShellFromLegacy(frame)
			scene.Shell = &shell
		case "menu":
			scene.Menus = append(scene.Menus, appMenuFromLegacy(frame, "popup"))
		case "dialog", "window":
			scene.Dialogs = append(scene.Dialogs, appDialogFromLegacy(frame))
		case "viewer", "editor", "terminal":
			surface := appSurfaceFromLegacy(frame)
			scene.Surface = &surface
		}
	}
	appAppendAutocompleteMenus(&scene, autocompletes)
	if scene.Shell != nil && scene.Surface == nil && scene.Shell.TerminalActive && scene.Shell.Terminal != nil {
		scene.Surface = &extui.SurfaceModel{
			ID:    scene.Shell.Terminal.ID,
			Kind:  "terminal",
			Title: scene.Shell.Terminal.Title,
			Rows:  scene.Shell.Terminal.Rows,
			Busy:  scene.Shell.Terminal.Busy,
		}
	}
	return scene.ToMap()
}

type appAutocompleteMenu struct {
	menu     *vtui.AutoCompleteMenu
	id       string
	windowID string
	x        int
	y        int
	w        int
	h        int
}

type appAutocompleteMenus []appAutocompleteMenu

func appActiveAutocompleteMenus() appAutocompleteMenus {
	if vtui.FrameManager == nil {
		return nil
	}
	var out appAutocompleteMenus
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		ac, ok := frame.(*vtui.AutoCompleteMenu)
		if !ok || !ac.HasMatches() {
			continue
		}
		x1, y1, x2, y2 := ac.GetPosition()
		out = append(out, appAutocompleteMenu{
			menu:     ac,
			id:       vtui.SemanticID(ac),
			windowID: vtui.SemanticID(&ac.Window),
			x:        x1,
			y:        y1,
			w:        x2 - x1 + 1,
			h:        y2 - y1 + 1,
		})
	}
	return out
}

func (menus appAutocompleteMenus) isLegacyFrame(frame map[string]any) bool {
	kind := semanticString(frame["kind"])
	if kind != "dialog" && kind != "window" {
		return false
	}
	id := semanticString(frame["id"])
	x := semanticInt(frame["x"])
	y := semanticInt(frame["y"])
	w := semanticInt(frame["w"])
	h := semanticInt(frame["h"])
	for _, item := range menus {
		if id != "" && (id == item.id || id == item.windowID) {
			return true
		}
		if x == item.x && y == item.y && w == item.w && h == item.h && semanticString(frame["title"]) == "" {
			return true
		}
	}
	return false
}

func appAppendAutocompleteMenus(scene *extui.Scene, autocompletes appAutocompleteMenus) {
	if scene == nil {
		return
	}

	for _, item := range autocompletes {
		ac := item.menu
		menu := extui.MenuModel{
			ID:       item.id,
			Role:     "autocomplete",
			Title:    "Autocomplete",
			Active:   true,
			Selected: 0,
			Legacy: extui.M{
				"x": item.x,
				"y": item.y,
				"w": item.w,
				"h": item.h,
			},
		}
		for i, match := range ac.Matches {
			menu.Items = append(menu.Items, extui.MenuItemModel{
				Index:   i,
				Text:    match,
				RawText: match,
			})
		}
		scene.Menus = append(scene.Menus, menu)
	}
}

func appShellFromLegacy(node map[string]any) extui.ShellModel {
	shell := extui.ShellModel{
		ID:             semanticString(node["id"]),
		Title:          semanticString(node["title"]),
		Mode:           "panels",
		ActivePanel:    semanticInt(node["activePanel"]),
		ShowPanels:     appBoolDefault(node["showPanels"], true),
		ShowKeyBar:     appBoolDefault(node["showKeyBar"], true),
		TerminalBusy:   appBool(node["terminalBusy"]),
		TerminalActive: appBool(node["terminalActive"]),
		MacroRecording: appBool(node["macroRecording"]),
	}
	if shell.TerminalActive {
		shell.Mode = "terminal"
	}
	for _, panel := range appMapSlice(node["panels"]) {
		shell.Panels = append(shell.Panels, appPanelFromLegacy(panel))
	}
	if cmd := appMap(node["commandLine"]); cmd != nil {
		c := appCommandLineFromLegacy(cmd)
		shell.CommandLine = &c
	}
	if term := appMap(node["terminal"]); term != nil {
		t := appTerminalFromLegacy(term)
		shell.Terminal = &t
	}
	return shell
}

func appPanelFromLegacy(node map[string]any) extui.PanelModel {
	panel := extui.PanelModel{
		ID:            semanticString(node["id"]),
		Side:          semanticInt(node["side"]),
		Active:        appBool(node["active"]),
		Path:          semanticString(node["path"]),
		Title:         semanticString(node["title"]),
		ViewMode:      semanticString(node["viewModeName"]),
		SortMode:      semanticString(node["sortModeName"]),
		SortReverse:   appBool(node["sortReverse"]),
		Cursor:        semanticInt(node["cursor"]),
		Top:           semanticInt(node["top"]),
		Loading:       appBool(node["loading"]),
		FastFind:      appBool(node["fastFind"]),
		FastFindText:  semanticString(node["fastFindText"]),
		SelectedCount: semanticInt(node["selectedCount"]),
		SelectedSize:  appInt64(node["selectedSize"]),
		TotalCount:    semanticInt(node["totalCount"]),
		TotalSize:     appInt64(node["totalSize"]),
	}
	for _, entry := range appMapSlice(node["entries"]) {
		panel.Entries = append(panel.Entries, appEntryFromLegacy(entry))
	}
	return panel
}

func appEntryFromLegacy(node map[string]any) extui.FileEntryModel {
	return extui.FileEntryModel{
		Index:          semanticInt(node["index"]),
		Name:           semanticString(node["name"]),
		Size:           appInt64(node["size"]),
		SizeText:       semanticString(node["sizeText"]),
		IsDir:          appBool(node["isDir"]),
		IsUp:           appBool(node["isUp"]),
		IsHidden:       appBool(node["isHidden"]),
		IsExecutable:   appBool(node["isExecutable"]),
		IsCached:       appBool(node["isCached"]),
		Selected:       appBool(node["selected"]),
		SizeCalculated: appBool(node["sizeCalculated"]),
		MTime:          semanticString(node["mtime"]),
		Mode:           semanticString(node["mode"]),
	}
}

func appCommandLineFromLegacy(node map[string]any) extui.CommandLineModel {
	return extui.CommandLineModel{
		ID:         semanticString(node["id"]),
		Visible:    appBoolDefault(node["visible"], true),
		Focused:    appBool(node["focused"]),
		Prompt:     semanticString(node["prompt"]),
		PromptRuns: appRunsFromLegacy(node["promptRuns"]),
		Text:       semanticString(node["text"]),
		Empty:      appBool(node["empty"]),
	}
}

func appTerminalFromLegacy(node map[string]any) extui.TerminalModel {
	term := extui.TerminalModel{
		ID:        semanticString(node["id"]),
		Title:     semanticString(node["title"]),
		Visible:   appBoolDefault(node["visible"], true),
		Focused:   appBool(node["focused"]),
		AltScreen: appBool(node["altScreen"]),
		Busy:      appBool(node["busy"]),
		CursorX:   semanticInt(node["cursorX"]),
		CursorY:   semanticInt(node["cursorY"]),
	}
	for _, row := range appMapSlice(node["rows"]) {
		term.Rows = append(term.Rows, appTextRowFromLegacy(row))
	}
	return term
}

func appSurfaceFromLegacy(node map[string]any) extui.SurfaceModel {
	surface := extui.SurfaceModel{
		ID:           semanticString(node["id"]),
		Kind:         semanticString(node["kind"]),
		Title:        semanticString(node["title"]),
		Path:         semanticString(node["path"]),
		BaseName:     semanticString(node["baseName"]),
		Mode:         semanticString(node["mode"]),
		Busy:         appBool(node["busy"]),
		Dirty:        appBool(node["dirty"]),
		Saving:       appBool(node["saving"]),
		HexMode:      appBool(node["hexMode"]),
		WrapMode:     appBool(node["wrapMode"]),
		WordWrap:     appBool(node["wordWrap"]),
		Overtype:     appBool(node["overtype"]),
		TopOffset:    appInt64(node["topOffset"]),
		Size:         appInt64(node["size"]),
		CursorLine:   semanticInt(node["cursorLine"]),
		CursorPos:    semanticInt(node["cursorPos"]),
		ScrollTop:    semanticInt(node["scrollTop"]),
		ScrollLeft:   semanticInt(node["scrollLeft"]),
		Selection:    appBool(node["selection"]),
		Autocomplete: appMap(node["autocomplete"]),
	}
	for _, row := range appMapSlice(node["rows"]) {
		surface.Rows = append(surface.Rows, appTextRowFromLegacy(row))
	}
	return surface
}

func appTextRowFromLegacy(node map[string]any) extui.TextRowModel {
	return extui.TextRowModel{
		Index:       semanticInt(node["index"]),
		VisualRow:   semanticInt(node["visualRow"]),
		LogicalLine: semanticInt(node["logicalLine"]),
		Offset:      appInt64(node["offset"]),
		Text:        semanticString(node["text"]),
		Runs:        appRunsFromLegacy(node["runs"]),
	}
}

func appMenuFromLegacy(node map[string]any, role string) extui.MenuModel {
	menu := extui.MenuModel{
		ID:       semanticString(node["id"]),
		Role:     role,
		Title:    semanticString(node["title"]),
		Active:   appBool(node["active"]),
		Selected: semanticInt(node["selected"]),
		Legacy:   node,
	}
	for _, item := range appMapSlice(node["items"]) {
		menu.Items = append(menu.Items, appMenuItemFromLegacy(item))
	}
	return menu
}

func appMenuItemFromLegacy(node map[string]any) extui.MenuItemModel {
	item := extui.MenuItemModel{
		Index:     semanticInt(node["index"]),
		Text:      semanticString(node["text"]),
		RawText:   semanticString(node["rawText"]),
		Hotkey:    semanticString(node["hotkey"]),
		Shortcut:  semanticString(node["shortcut"]),
		Command:   semanticInt(node["command"]),
		Separator: appBool(node["separator"]),
		Disabled:  appBool(node["disabled"]),
		Legacy:    node,
	}
	for _, child := range appMapSlice(node["items"]) {
		item.Items = append(item.Items, appMenuItemFromLegacy(child))
	}
	return item
}

func appKeyBarFromLegacy(node map[string]any) extui.KeyBarModel {
	keyBar := extui.KeyBarModel{
		ID:       semanticString(node["id"]),
		Visible:  appBoolDefault(node["visible"], true),
		Modifier: semanticString(node["modifier"]),
	}
	for _, item := range appMapSlice(node["items"]) {
		keyBar.Items = append(keyBar.Items, extui.KeyBarItemModel{
			Index: semanticInt(item["index"]),
			Key:   semanticString(item["key"]),
			Text:  semanticString(item["text"]),
		})
	}
	return keyBar
}

func appDialogFromLegacy(node map[string]any) extui.DialogModel {
	dlg := extui.DialogModel{
		ID:        semanticString(node["id"]),
		Kind:      semanticString(node["kind"]),
		Title:     semanticString(node["title"]),
		Modal:     appBool(node["modal"]),
		Busy:      appBool(node["busy"]),
		Progress:  semanticInt(node["progress"]),
		ShowClose: appBool(node["showClose"]),
		Legacy:    node,
	}
	for _, child := range appMapSlice(node["children"]) {
		dlg.Controls = append(dlg.Controls, appControlFromLegacy(child))
	}
	return dlg
}

func appControlFromLegacy(node map[string]any) extui.ControlModel {
	ctrl := extui.ControlModel{
		ID:         semanticString(node["id"]),
		Kind:       semanticString(node["kind"]),
		Visible:    appBoolDefault(node["visible"], true),
		Focused:    appBool(node["focused"]),
		Disabled:   appBool(node["disabled"]),
		Text:       semanticString(node["text"]),
		Title:      semanticString(node["title"]),
		Hotkey:     semanticString(node["hotkey"]),
		State:      semanticInt(node["state"]),
		ThreeState: appBool(node["threeState"]),
		Default:    appBool(node["default"]),
		Password:   appBool(node["password"]),
		Cursor:     semanticInt(node["cursor"]),
		Left:       semanticInt(node["left"]),
		Selected:   appIntSlice(node["selected"]),
		Items:      appStringSlice(node["items"]),
		Rows:       appMapSlice(node["rows"]),
		Legacy:     node,
	}
	for _, child := range appMapSlice(node["children"]) {
		ctrl.Children = append(ctrl.Children, appControlFromLegacy(child))
	}
	return ctrl
}

func appRunsFromLegacy(value any) []extui.RunModel {
	var runs []extui.RunModel
	for _, run := range appMapSlice(value) {
		runs = append(runs, extui.RunModel{
			Text: semanticString(run["text"]),
			Attr: uint64(appInt64(run["attr"])),
		})
	}
	return runs
}

func appMap(value any) map[string]any {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return nil
}

func appMapSlice(value any) []map[string]any {
	switch items := value.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if m := appMap(item); m != nil {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func appStringSlice(value any) []string {
	switch items := value.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			out = append(out, semanticString(item))
		}
		return out
	default:
		return nil
	}
}

func appIntSlice(value any) []int {
	switch items := value.(type) {
	case []int:
		return items
	case []any:
		out := make([]int, 0, len(items))
		for _, item := range items {
			out = append(out, semanticInt(item))
		}
		return out
	default:
		return nil
	}
}

func appBool(value any) bool {
	b, _ := value.(bool)
	return b
}

func appBoolDefault(value any, fallback bool) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return fallback
}

func appInt64(value any) int64 {
	switch n := value.(type) {
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
		if n > uint64(^uint(0)>>1) {
			return 0
		}
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
