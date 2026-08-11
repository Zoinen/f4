package main

import (
	"strings"

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
	vmenus := appActiveVMenus()
	semanticElements := appActiveSemanticElements()
	scene := extui.Scene{
		Width:          semanticInt(legacy["width"]),
		Height:         semanticInt(legacy["height"]),
		ActiveScreen:   semanticInt(legacy["activeScreen"]),
		WorkspaceCount: semanticInt(legacy["workspaceCount"]),
		WorkspaceTabs:  appMap(legacy["workspaceTabs"]),
		Presentation:   string(parseGuiPresentationMode(string(AppConfig.GuiPresentation))),
		QmlIconSet:     string(parseQmlIconSetMode(string(AppConfig.QmlIconSet))),
		Legacy:         appLegacyWithoutVMenus(legacy, vmenus),
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
		if autocompletes.isLegacyFrame(frame) || vmenus.isLegacyFrame(frame) {
			continue
		}
		switch semanticString(frame["kind"]) {
		case "panels", "shell":
			shell := appShellFromLegacy(frame)
			scene.Shell = &shell
		case "menu":
			scene.Menus = append(scene.Menus, appMenuFromLegacy(frame, "popup"))
		case "dialog", "window":
			appEnrichLegacyTextWidgets(frame, semanticElements)
			scene.Dialogs = append(scene.Dialogs, appDialogFromLegacy(frame))
		case "viewer", "editor", "terminal":
			surface := appSurfaceFromLegacy(frame)
			scene.Surface = &surface
		}
	}
	for _, menu := range vmenus {
		scene.Menus = append(scene.Menus, menu.model())
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

// vtui.Text and a few other passive labels do not yet implement
// vtui.SemanticProvider. The generic exporter therefore preserves their
// geometry but emits only kind=widget, without the text that a native UI needs
// to render. Match those nodes back to the live, authoritative UI tree while
// adapting the scene. This keeps the compatibility scene untouched for every
// interactive control and can be removed once vtui exports labels itself.
func appActiveSemanticElements() map[string]vtui.UIElement {
	elements := make(map[string]vtui.UIElement)
	if vtui.FrameManager == nil {
		return elements
	}
	var add func(vtui.UIElement)
	add = func(element vtui.UIElement) {
		if element == nil {
			return
		}
		elements[vtui.SemanticID(element)] = element
		if container, ok := element.(vtui.Container); ok {
			for _, child := range container.GetChildren() {
				add(child)
			}
		}
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		if container, ok := frame.(vtui.Container); ok {
			for _, child := range container.GetChildren() {
				add(child)
			}
		}
	}
	return elements
}

func appEnrichLegacyTextWidgets(node map[string]any, elements map[string]vtui.UIElement) {
	if node == nil {
		return
	}
	if semanticString(node["kind"]) == "widget" {
		if element := elements[semanticString(node["id"])]; element != nil {
			switch label := element.(type) {
			case *vtui.Text:
				text, hotkey, _ := vtui.ParseAmpersandString(label.GetText())
				node["kind"] = "text"
				node["text"] = text
				if hotkey != 0 {
					node["hotkey"] = string(hotkey)
				}
			case *vtui.DynamicText:
				text, hotkey, _ := vtui.ParseAmpersandString(label.GetText())
				node["kind"] = "text"
				node["text"] = text
				if hotkey != 0 {
					node["hotkey"] = string(hotkey)
				}
			case *vtui.VText:
				node["kind"] = "text"
				node["text"] = label.Content
			}
		}
	}
	for _, child := range appMapSlice(node["children"]) {
		appEnrichLegacyTextWidgets(child, elements)
	}
}

type appVMenu struct {
	frame          vtui.Frame
	menu           *vtui.VMenu
	bottomHint     string
	menuBarSubmenu bool
}

type appVMenus []appVMenu

func appNormalizeMenuCheckmark(text string) (string, bool) {
	for _, marker := range []string{"√", "✓"} {
		if strings.HasPrefix(text, marker) {
			return strings.TrimPrefix(strings.TrimPrefix(text, marker), " "), true
		}
	}
	return text, false
}

func appActiveVMenus() appVMenus {
	if vtui.FrameManager == nil {
		return nil
	}
	var out appVMenus
	menuBar := vtui.FrameManager.GetActiveMenuBar()
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		menu, bottomHint := appFrameVMenu(frame)
		if menu != nil {
			out = append(out, appVMenu{
				frame:          frame,
				menu:           menu,
				bottomHint:     bottomHint,
				menuBarSubmenu: appIsMenuBarSubmenu(frame, menuBar),
			})
		}
	}
	return out
}

func appIsMenuBarSubmenu(frame vtui.Frame, menuBar *vtui.MenuBar) bool {
	if frame == nil || menuBar == nil || !menuBar.Active || len(menuBar.Items) == 0 {
		return false
	}
	selected := menuBar.SelectPos
	if selected < 0 || selected >= len(menuBar.Items) {
		return false
	}
	x1, y1, _, _ := frame.GetPosition()
	return x1 == menuBar.GetItemX(selected) && y1 == menuBar.Y1+1
}

func appFrameVMenu(frame vtui.Frame) (*vtui.VMenu, string) {
	switch item := frame.(type) {
	case *vtui.VMenu:
		return item, ""
	case *bookmarksFrame:
		return item.VMenu, item.bottomHint
	case *userMenuFrame:
		return item.VMenu, item.bottomHint
	default:
		return nil, ""
	}
}

func (item appVMenu) model() extui.MenuModel {
	x1, y1, x2, y2 := item.frame.GetPosition()
	menu := extui.MenuModel{
		ID:       vtui.SemanticID(item.frame),
		Role:     "vmenu",
		Title:    item.frame.GetTitle(),
		Active:   true,
		Selected: item.menu.SelectPos,
		Legacy: extui.M{
			"x":              x1,
			"y":              y1,
			"w":              x2 - x1 + 1,
			"h":              y2 - y1 + 1,
			"top":            item.menu.TopPos,
			"viewHeight":     item.menu.ViewHeight,
			"bottomHint":     item.bottomHint,
			"shadow":         item.frame.HasShadow(),
			"menuBarSubmenu": item.menuBarSubmenu,
		},
	}
	for i, source := range item.menu.Items {
		clean, hotkey, _ := vtui.ParseAmpersandString(source.Text)
		clean, checked := appNormalizeMenuCheckmark(clean)
		hotkeyText := ""
		if hotkey != 0 {
			hotkeyText = string(hotkey)
		}
		disabled := false
		if vtui.FrameManager != nil {
			disabled = vtui.FrameManager.DisabledCommands.IsDisabled(source.Command)
		}
		menu.Items = append(menu.Items, extui.MenuItemModel{
			Index:     i,
			Text:      clean,
			RawText:   source.Text,
			Hotkey:    hotkeyText,
			Shortcut:  source.Shortcut,
			Command:   source.Command,
			Separator: source.Separator,
			Disabled:  disabled,
			Checked:   checked,
		})
	}
	return menu
}

func (menus appVMenus) isLegacyFrame(frame map[string]any) bool {
	id := semanticString(frame["id"])
	if id == "" {
		return false
	}
	for _, menu := range menus {
		if id == vtui.SemanticID(menu.frame) {
			return true
		}
	}
	return false
}

func appLegacyWithoutVMenus(legacy map[string]any, menus appVMenus) map[string]any {
	if len(menus) == 0 {
		return legacy
	}
	out := make(map[string]any, len(legacy))
	for key, value := range legacy {
		out[key] = value
	}
	frames := appMapSlice(legacy["frames"])
	filtered := make([]map[string]any, 0, len(frames))
	for _, frame := range frames {
		if !menus.isLegacyFrame(frame) {
			filtered = append(filtered, frame)
		}
	}
	out["frames"] = filtered
	return out
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
		scene.Menus = append(scene.Menus, item.model())
	}
}

func (item appAutocompleteMenu) model() extui.MenuModel {
	menu := extui.MenuModel{
		ID:     item.id,
		Role:   "autocomplete",
		Title:  "Autocomplete",
		Active: true,
		// Autocomplete is an offer, not an implicit edit.  In particular an
		// exact prefix match must not replace what the user typed when Enter is
		// pressed.  QML owns this transient selection and starts it only after
		// an explicit Up/Down or pointer gesture.
		Selected: -1,
		Legacy: extui.M{
			"x":     item.x,
			"y":     item.y,
			"w":     item.w,
			"h":     item.h,
			"query": item.menu.Edit.GetText(),
		},
	}
	for i, match := range item.menu.Matches {
		menu.Items = append(menu.Items, extui.MenuItemModel{
			Index:   i,
			Text:    match,
			RawText: match,
		})
	}
	return menu
}

func appShellFromLegacy(node map[string]any) extui.ShellModel {
	shell := extui.ShellModel{
		ID:             semanticString(node["id"]),
		Title:          semanticString(node["title"]),
		Mode:           "panels",
		ActivePanel:    semanticInt(node["activePanel"]),
		ShowPanels:     appBoolDefault(node["showPanels"], true),
		ShowLeftPanel:  appBoolDefault(node["showLeftPanel"], true),
		ShowRightPanel: appBoolDefault(node["showRightPanel"], true),
		Wide:           appBool(node["wide"]),
		WidePanel:      semanticInt(node["widePanel"]),
		ShowKeyBar:     appBoolDefault(node["showKeyBar"], true),
		TerminalBusy:   appBool(node["terminalBusy"]),
		TerminalActive: appBool(node["terminalActive"]),
		MacroRecording: appBool(node["macroRecording"]),
		Fallback:       appBool(node["fallback"]),
		FallbackReason: semanticString(node["reason"]),
	}
	if shell.TerminalActive {
		shell.Mode = "terminal"
	}
	for _, panel := range appMapSlice(node["panels"]) {
		shell.Panels = append(shell.Panels, appPanelFromLegacy(panel))
	}
	for _, panel := range appMapSlice(node["infoPanels"]) {
		shell.InfoPanels = append(shell.InfoPanels, appInfoPanelFromLegacy(panel))
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

func appInfoPanelFromLegacy(node map[string]any) extui.InfoPanelModel {
	panel := extui.InfoPanelModel{
		ID:         semanticString(node["id"]),
		Side:       semanticInt(node["side"]),
		Active:     appBool(node["active"]),
		Title:      semanticString(node["title"]),
		BottomHint: semanticString(node["bottomHint"]),
	}
	for _, row := range appMapSlice(node["rows"]) {
		panel.Rows = append(panel.Rows, extui.InfoPanelRowModel{
			Kind:  semanticString(row["kind"]),
			Label: semanticString(row["label"]),
			Value: semanticString(row["value"]),
		})
	}
	return panel
}

func appPanelFromLegacy(node map[string]any) extui.PanelModel {
	presentation := semanticString(node["presentation"])
	if presentation == "" {
		presentation = string(PanelPresentationList)
	}
	sourceKind := semanticString(node["sourceKind"])
	if sourceKind == "" {
		sourceKind = "vfs"
	}
	galleryLayoutMode := semanticString(node["galleryLayoutMode"])
	if _, ok := parseGalleryLayoutMode(galleryLayoutMode); !ok {
		galleryLayoutMode = string(GalleryLayoutMasonry)
	}
	galleryColumnCount := semanticInt(node["galleryColumnCount"])
	if galleryColumnCount < minGalleryColumnCount || galleryColumnCount > maxGalleryColumnCount {
		galleryColumnCount = defaultGalleryColumnCount
	}
	parsedGalleryLayoutMode, _ := parseGalleryLayoutMode(galleryLayoutMode)
	galleryDensity := semanticInt(node["galleryDensity"])
	if galleryDensity > 0 {
		galleryDensity = clampGalleryDensity(parsedGalleryLayoutMode, galleryDensity)
	} else {
		galleryDensity, _, _ = galleryDensityLimits(parsedGalleryLayoutMode)
	}
	galleryLayoutRevision := appInt64(node["galleryLayoutRevision"])
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	panel := extui.PanelModel{
		ID:                     semanticString(node["id"]),
		Side:                   semanticInt(node["side"]),
		Active:                 appBool(node["active"]),
		Path:                   semanticString(node["path"]),
		Title:                  semanticString(node["title"]),
		ViewMode:               semanticString(node["viewModeName"]),
		Presentation:           presentation,
		GalleryLayoutMode:      galleryLayoutMode,
		GalleryColumnCount:     galleryColumnCount,
		GalleryDensity:         galleryDensity,
		GalleryLayoutRevision:  galleryLayoutRevision,
		SourceKind:             sourceKind,
		PreviewCapable:         appBool(node["previewCapable"]),
		CatalogRevision:        appInt64(node["catalogRevision"]),
		SelectionRevision:      appInt64(node["selectionRevision"]),
		HighlightRevision:      appInt64(node["highlightRevision"]),
		CursorEntryID:          semanticString(node["cursorEntryId"]),
		SortMode:               semanticString(node["sortModeName"]),
		SortReverse:            appBool(node["sortReverse"]),
		SeparateFileExtensions: appBool(node["separateFileExtensions"]),
		Cursor:                 semanticInt(node["cursor"]),
		Top:                    semanticInt(node["top"]),
		Loading:                appBool(node["loading"]),
		FastFind:               appBool(node["fastFind"]),
		FastFindText:           semanticString(node["fastFindText"]),
		SelectedCount:          semanticInt(node["selectedCount"]),
		SelectedSize:           appInt64(node["selectedSize"]),
		TotalCount:             semanticInt(node["totalCount"]),
		TotalSize:              appInt64(node["totalSize"]),
	}
	parseColumns := func(value any) []extui.PanelColumnModel {
		columns := appMapSlice(value)
		result := make([]extui.PanelColumnModel, 0, len(columns))
		for _, column := range columns {
			result = append(result, extui.PanelColumnModel{
				ID:        semanticString(column["id"]),
				Role:      semanticString(column["role"]),
				Index:     semanticInt(column["index"]),
				Title:     semanticString(column["title"]),
				Width:     semanticInt(column["width"]),
				Alignment: semanticString(column["alignment"]),
				SortMode:  semanticString(column["sortMode"]),
				Sortable:  appBool(column["sortable"]),
			})
		}
		return result
	}
	panel.Columns = parseColumns(node["columns"])
	panel.GalleryColumns = parseColumns(node["galleryColumns"])
	if rawStyles, ok := node["highlightStyles"].(map[string]any); ok {
		panel.HighlightStyles = make(map[string]extui.HighlightStyleModel, len(rawStyles))
		for id, rawStyle := range rawStyles {
			panel.HighlightStyles[id] = appHighlightStyleFromLegacy(appMap(rawStyle))
		}
	}
	for _, entry := range appMapSlice(node["entries"]) {
		panel.Entries = append(panel.Entries, appEntryFromLegacy(entry))
	}
	return panel
}

func appEntryFromLegacy(node map[string]any) extui.FileEntryModel {
	return extui.FileEntryModel{
		Index:            semanticInt(node["index"]),
		EntryID:          semanticString(node["entryId"]),
		Name:             semanticString(node["name"]),
		DisplayBaseName:  semanticString(node["displayBaseName"]),
		DisplayExtension: semanticString(node["displayExtension"]),
		LocalPath:        semanticString(node["localPath"]),
		Size:             appInt64(node["size"]),
		SizeText:         semanticString(node["sizeText"]),
		IsDir:            appBool(node["isDir"]),
		IsUp:             appBool(node["isUp"]),
		IsHidden:         appBool(node["isHidden"]),
		IsExecutable:     appBool(node["isExecutable"]),
		IsCached:         appBool(node["isCached"]),
		Selected:         appBool(node["selected"]),
		SizeCalculated:   appBool(node["sizeCalculated"]),
		MTime:            semanticString(node["mtime"]),
		MTimeNanos:       appInt64(node["mtimeNanos"]),
		Version:          semanticString(node["version"]),
		Mode:             semanticString(node["mode"]),
		HighlightStyleID: semanticString(node["highlightStyleId"]),
	}
}

func appHighlightStyleFromLegacy(node map[string]any) extui.HighlightStyleModel {
	patch := func(value any) extui.HighlightColorPatchModel {
		m := appMap(value)
		return extui.HighlightColorPatchModel{
			Foreground: semanticString(m["foreground"]),
			Background: semanticString(m["background"]),
		}
	}
	style := extui.HighlightStyleModel{
		Marker:         semanticString(node["marker"]),
		Icon:           semanticString(node["icon"]),
		Normal:         patch(node["normal"]),
		Selected:       patch(node["selected"]),
		Cursor:         patch(node["cursor"]),
		SelectedCursor: patch(node["selectedCursor"]),
	}
	for _, group := range appMapSlice(node["groups"]) {
		style.Groups = append(style.Groups, extui.HighlightGroupModel{
			ID:   semanticString(group["id"]),
			Name: semanticString(group["name"]),
		})
	}
	return style
}

func appCommandLineFromLegacy(node map[string]any) extui.CommandLineModel {
	return extui.CommandLineModel{
		ID:               semanticString(node["id"]),
		Visible:          appBoolDefault(node["visible"], true),
		Focused:          appBool(node["focused"]),
		Prompt:           semanticString(node["prompt"]),
		PromptRuns:       appRunsFromLegacy(node["promptRuns"]),
		Text:             semanticString(node["text"]),
		Empty:            appBool(node["empty"]),
		Runs:             appRunsFromLegacy(node["runs"]),
		InputX:           semanticInt(node["inputX"]),
		CursorPrefixRuns: appRunsFromLegacy(node["cursorPrefixRuns"]),
		CursorX:          semanticInt(node["cursorX"]),
		CursorVisible:    appBool(node["cursorVisible"]),
		CursorShape:      semanticString(node["cursorShape"]),
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
		ID:                 semanticString(node["id"]),
		Kind:               semanticString(node["kind"]),
		Title:              semanticString(node["title"]),
		Path:               semanticString(node["path"]),
		BaseName:           semanticString(node["baseName"]),
		Mode:               semanticString(node["mode"]),
		Busy:               appBool(node["busy"]),
		Dirty:              appBool(node["dirty"]),
		Saving:             appBool(node["saving"]),
		HexMode:            appBool(node["hexMode"]),
		WrapMode:           appBool(node["wrapMode"]),
		WordWrap:           appBool(node["wordWrap"]),
		Overtype:           appBool(node["overtype"]),
		TopOffset:          appInt64(node["topOffset"]),
		Size:               appInt64(node["size"]),
		CursorLine:         semanticInt(node["cursorLine"]),
		CursorPos:          semanticInt(node["cursorPos"]),
		CursorVisualRow:    semanticInt(node["cursorVisualRow"]),
		CursorVisualColumn: semanticInt(node["cursorVisualColumn"]),
		CursorVisible:      appBool(node["cursorVisible"]),
		CursorShape:        semanticString(node["cursorShape"]),
		ScrollTop:          semanticInt(node["scrollTop"]),
		ScrollLeft:         semanticInt(node["scrollLeft"]),
		Selection:          appBool(node["selection"]),
		Autocomplete:       appMap(node["autocomplete"]),
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
	text, checked := appNormalizeMenuCheckmark(semanticString(node["text"]))
	item := extui.MenuItemModel{
		Index:     semanticInt(node["index"]),
		Text:      text,
		RawText:   semanticString(node["rawText"]),
		Hotkey:    semanticString(node["hotkey"]),
		Shortcut:  semanticString(node["shortcut"]),
		Command:   semanticInt(node["command"]),
		Separator: appBool(node["separator"]),
		Disabled:  appBool(node["disabled"]),
		Checked:   checked || appBool(node["checked"]),
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
			Text:       semanticString(run["text"]),
			Attr:       uint64(appInt64(run["attr"])),
			Foreground: semanticString(run["foreground"]),
			Background: semanticString(run["background"]),
			Bold:       appBool(run["bold"]),
			Underline:  appBool(run["underline"]),
			Strikeout:  appBool(run["strikeout"]),
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
