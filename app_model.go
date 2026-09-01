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
		WorkspaceTabs:  appQueueAwareWorkspaceTabs(legacy),
		Presentation:   string(parseGuiPresentationMode(string(AppConfig.GuiPresentation))),
		QmlIconSet:     string(parseQmlIconSetMode(string(AppConfig.QmlIconSet))),
		Legacy:         appLegacyForNativeScene(legacy, vmenus, autocompletes),
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
		case "operationsQueue":
			queue := appOperationsQueueFromLegacy(frame)
			queue.WorkspaceIndex = scene.ActiveScreen
			appBindOperationsQueueWorkspace(&queue, scene.WorkspaceTabs)
			scene.OperationsQueue = &queue
		}
	}
	for _, menu := range vmenus {
		scene.Menus = append(scene.Menus, menu.model())
	}
	appAppendAutocompleteMenus(&scene, autocompletes)
	commitMediaPanelsForSemanticScene(legacy)
	return scene.ToMap()
}

func semanticMediaPanelIDs(legacy map[string]any) []string {
	seen := make(map[string]struct{})
	panelIDs := make([]string, 0, 4)
	collectFrames := func(frames []map[string]any) {
		for _, frame := range frames {
			kind := semanticString(frame["kind"])
			if kind != "panels" && kind != "shell" {
				continue
			}
			for _, panel := range appMapSlice(frame["panels"]) {
				panelID := semanticString(panel["id"])
				if panelID == "" {
					continue
				}
				if _, duplicate := seen[panelID]; duplicate {
					continue
				}
				seen[panelID] = struct{}{}
				panelIDs = append(panelIDs, panelID)
			}
		}
	}

	// screens is the authoritative complete workspace set. frames is retained
	// as a compatibility fallback not only for callers constructing a legacy
	// scene by hand without a screens field, but also for the older live
	// renderer which exports screen frame shells without copying the nested
	// panel list into each screen frame.  In that case an empty result from
	// screens must not retire every freshly registered broker resource.
	if screens, present := legacy["screens"]; present {
		for _, screen := range appMapSlice(screens) {
			collectFrames(appMapSlice(screen["frames"]))
		}
	}
	if len(panelIDs) == 0 {
		collectFrames(appMapSlice(legacy["frames"]))
	}
	return panelIDs
}

func commitMediaPanelsForSemanticScene(legacy map[string]any) {
	if broker := currentExtUiMediaBroker(); broker != nil {
		broker.CommitScenePanels(semanticMediaPanelIDs(legacy))
	}
}

// appQueueAwareWorkspaceTabs keeps vtui's stable workspace identity while
// making the queue's close guard explicit to every native tab renderer.  The
// semantic action handler repeats the guard authoritatively, so a stale scene
// cannot close a queue whose task became active in the meantime.
func appQueueAwareWorkspaceTabs(legacy map[string]any) map[string]any {
	source := appMap(legacy["workspaceTabs"])
	if source == nil {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	tabs := appMapSlice(source["tabs"])
	clonedTabs := make([]map[string]any, 0, len(tabs))
	for _, tab := range tabs {
		clone := make(map[string]any, len(tab))
		for key, value := range tab {
			clone[key] = value
		}
		clonedTabs = append(clonedTabs, clone)
	}

	for screenIndex, screen := range appMapSlice(legacy["screens"]) {
		queueCanClose := true
		surfaceKind, iconName := appWorkspaceTabPresentation(screen)
		if surfaceKind == "operationsQueue" {
			for _, frame := range appMapSlice(screen["frames"]) {
				if semanticString(frame["kind"]) == "operationsQueue" {
					queueCanClose = appBool(frame["canClose"])
					break
				}
			}
		}
		screenNumber := semanticInt(screen["number"])
		for _, tab := range clonedTabs {
			if semanticInt(tab["index"]) == screenIndex ||
				(screenNumber > 0 && semanticInt(tab["number"]) == screenNumber) {
				if surfaceKind != "" {
					tab["surfaceKind"] = surfaceKind
					tab["iconName"] = iconName
					if _, structuredMarker := tab["marker"]; structuredMarker {
						// Current semantic scenes already split the frame kind
						// into marker. Preserve legitimate titles such as
						// "P projects" instead of treating their first letter
						// as an old cell-renderer prefix.
						tab["text"] = strings.TrimSpace(semanticString(tab["text"]))
					} else {
						tab["text"] = appWorkspaceTabText(semanticString(tab["text"]))
					}
				}
				if surfaceKind == "operationsQueue" {
					tab["closable"] = queueCanClose
				}
				break
			}
		}
	}
	out["tabs"] = clonedTabs
	return out
}

// appWorkspaceTabPresentation turns the terminal renderer's decorative title
// prefix into typed GUI metadata. The QML tab bar can then render a real
// Lucide icon without guessing from a localized title or retaining Unicode
// pseudo-icons that were designed for a cell grid.
func appWorkspaceTabPresentation(screen map[string]any) (kind, iconName string) {
	frames := appMapSlice(screen["frames"])
	for index := len(frames) - 1; index >= 0; index-- {
		frame := frames[index]
		switch semanticString(frame["kind"]) {
		case "operationsQueue":
			return "operationsQueue", "list-checks"
		case "editor":
			return "editor", "file-pen-line"
		case "viewer":
			return "viewer", "file-text"
		case "terminal":
			return "terminal", "square-terminal"
		case "panels", "shell":
			terminalActive := appBool(frame["terminalActive"])
			if showPanels, present := frame["showPanels"]; present && !appBool(showPanels) {
				terminalActive = true
			}
			if terminalActive {
				return "terminal", "square-terminal"
			}
			return "panels", "panels-top-left"
		}
	}
	return "", ""
}

func appWorkspaceTabText(title string) string {
	title = strings.TrimSpace(title)
	for _, prefix := range []string{"📁", "⌨", "👁", "✎"} {
		if strings.HasPrefix(title, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(title, prefix))
		}
	}
	// vtui's compact terminal tab bar encodes the frame kind next to the tab
	// number (for example "2V report.txt"). The semantic model keeps the
	// number separate and exports the marker at the start of text. Native tabs
	// already have typed icons, so retain only the content title here.
	for _, marker := range []string{"P", "V", "E", "T"} {
		if title == marker {
			return ""
		}
		if strings.HasPrefix(title, marker+" ") {
			return strings.TrimSpace(strings.TrimPrefix(title, marker))
		}
	}
	return title
}

func appBindOperationsQueueWorkspace(queue *extui.OperationsQueueModel, workspaceTabs map[string]any) {
	if queue == nil {
		return
	}
	for _, tab := range appMapSlice(workspaceTabs["tabs"]) {
		if semanticInt(tab["index"]) != queue.WorkspaceIndex && !appBool(tab["active"]) {
			continue
		}
		queue.TabID = semanticString(tab["id"])
		queue.WorkspaceNumber = semanticInt(tab["number"])
		return
	}
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
	if parent := item.menu.ParentMenu(); parent != nil {
		menu.ParentID = vtui.SemanticID(parent)
		menu.AnchorIndex = item.menu.ParentIndex()
	}
	for i, source := range item.menu.Items {
		clean, hotkey, _ := vtui.ParseAmpersandString(source.Text)
		clean, checked := appNormalizeMenuCheckmark(clean)
		hotkeyText := ""
		if hotkey != 0 {
			hotkeyText = string(hotkey)
		}
		disabled := source.Disabled
		if vtui.FrameManager != nil {
			disabled = disabled || vtui.FrameManager.DisabledCommands.IsDisabled(source.Command)
		}
		menu.Items = append(menu.Items, extui.MenuItemModel{
			Index:      i,
			ID:         source.ID,
			Text:       clean,
			RawText:    source.Text,
			Hotkey:     hotkeyText,
			Icon:       source.Icon,
			IconColor:  source.IconColor,
			Shortcut:   source.Shortcut,
			Command:    source.Command,
			Separator:  source.Separator,
			Header:     source.Header,
			Disabled:   disabled,
			Checked:    checked,
			HasSubmenu: source.Submenu != nil,
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

// The native app scene promotes shell command state and popup menus into
// dedicated typed fields. Keeping the same mutable objects in the legacy
// fallback duplicates them in every MessagePack scene and makes a single
// Backspace look like an unrelated full-scene mutation. Retain every genuine
// fallback frame while removing only those promoted duplicates.
func appLegacyForNativeScene(legacy map[string]any, menus appVMenus,
	autocompletes appAutocompleteMenus) map[string]any {
	if legacy == nil {
		return nil
	}
	out := make(map[string]any, len(legacy))
	for key, value := range legacy {
		out[key] = value
	}
	stripFrame := func(frame map[string]any) map[string]any {
		kind := semanticString(frame["kind"])
		if kind != "panels" && kind != "shell" {
			return frame
		}
		copyFrame := make(map[string]any, len(frame))
		for key, value := range frame {
			if key == "commandLine" {
				continue
			}
			if key == "panels" {
				panels := appMapSlice(value)
				lightweight := make([]map[string]any, 0, len(panels))
				for _, panel := range panels {
					if !appBool(panel["metadataDeferred"]) {
						lightweight = append(lightweight, panel)
						continue
					}
					panelCopy := make(map[string]any, len(panel))
					for panelKey, panelValue := range panel {
						switch panelKey {
						case "entries", "highlightStyles", "highlightRevision", "selectedSize", "totalSize":
							continue
						default:
							panelCopy[panelKey] = panelValue
						}
					}
					lightweight = append(lightweight, panelCopy)
				}
				copyFrame[key] = lightweight
				continue
			}
			copyFrame[key] = value
		}
		return copyFrame
	}
	filterFrames := func(value any) []map[string]any {
		frames := appMapSlice(value)
		filtered := make([]map[string]any, 0, len(frames))
		for _, frame := range frames {
			if menus.isLegacyFrame(frame) || autocompletes.isLegacyFrame(frame) {
				continue
			}
			filtered = append(filtered, stripFrame(frame))
		}
		return filtered
	}

	out["frames"] = filterFrames(legacy["frames"])
	if screens := appMapSlice(legacy["screens"]); len(screens) > 0 {
		strippedScreens := make([]map[string]any, 0, len(screens))
		for _, screen := range screens {
			screenCopy := make(map[string]any, len(screen))
			for key, value := range screen {
				if key == "frames" {
					screenCopy[key] = filterFrames(value)
				} else {
					screenCopy[key] = value
				}
			}
			strippedScreens = append(strippedScreens, screenCopy)
		}
		out["screens"] = strippedScreens
	}
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
	for _, panel := range appMapSlice(node["quickViews"]) {
		shell.QuickViews = append(shell.QuickViews, appQuickViewFromLegacy(panel))
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

func appQuickViewFromLegacy(node map[string]any) extui.QuickViewModel {
	view := extui.QuickViewModel{
		ID:          semanticString(node["id"]),
		Side:        semanticInt(node["side"]),
		SourceSide:  semanticInt(node["sourceSide"]),
		Active:      appBool(node["active"]),
		Title:       semanticString(node["title"]),
		BottomHint:  semanticString(node["bottomHint"]),
		ContentKey:  semanticString(node["contentKey"]),
		Name:        semanticString(node["name"]),
		Path:        semanticString(node["path"]),
		SizeText:    semanticString(node["sizeText"]),
		PreviewKind: semanticString(node["previewKind"]),
		Label:       semanticString(node["label"]),
		Error:       semanticString(node["error"]),
		Loading:     appBool(node["loading"]),
		Wrap:        appBool(node["wrap"]),
		ImageSource: semanticString(node["imageSource"]),
		ImageWidth:  semanticInt(node["imageWidth"]),
		ImageHeight: semanticInt(node["imageHeight"]),
	}
	for _, row := range appMapSlice(node["headerRows"]) {
		view.HeaderRows = append(view.HeaderRows, appTextRowFromLegacy(row))
	}
	if surface := appMap(node["surface"]); surface != nil {
		view.Surface = appSurfaceFromLegacy(surface)
	}
	return view
}

func appPanelFromLegacy(node map[string]any) extui.PanelModel {
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
	galleryDensities := make(map[string]int, len(galleryLayoutModes))
	suppliedGalleryDensities := appMap(node["galleryDensities"])
	for _, mode := range galleryLayoutModes {
		density := semanticInt(suppliedGalleryDensities[string(mode)])
		if density <= 0 {
			density, _, _ = galleryDensityLimits(mode)
			if density <= 0 {
				// The untouched compact default is derived from exact frontend
				// font metrics, so it remains absent until the user zooms.
				continue
			}
		}
		galleryDensities[string(mode)] = clampGalleryDensity(mode, density)
	}
	if galleryDensity > 0 {
		galleryDensities[string(parsedGalleryLayoutMode)] = galleryDensity
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
		ShowFileInfo:           appBool(node["showFileInfo"]),
		GalleryLayoutMode:      galleryLayoutMode,
		GalleryColumnCount:     galleryColumnCount,
		GalleryDensity:         galleryDensity,
		GalleryDensities:       galleryDensities,
		GalleryLayoutRevision:  galleryLayoutRevision,
		SourceKind:             sourceKind,
		PreviewCapable:         appBool(node["previewCapable"]),
		CatalogRevision:        appInt64(node["catalogRevision"]),
		SelectionRevision:      appInt64(node["selectionRevision"]),
		MetadataDeferred:       appBool(node["metadataDeferred"]),
		MetadataRevision:       appInt64(node["metadataRevision"]),
		HighlightRevision:      appInt64(node["highlightRevision"]),
		CursorEntryID:          semanticString(node["cursorEntryId"]),
		SortMode:               semanticString(node["sortModeName"]),
		SortReverse:            appBool(node["sortReverse"]),
		SeparateFileExtensions: appBool(node["separateFileExtensions"]),
		Cursor:                 semanticInt(node["cursor"]),
		Loading:                appBool(node["loading"]),
		CatalogProvisional:     appBool(node["catalogProvisional"]),
		FastFind:               appBool(node["fastFind"]),
		FastFindText:           semanticString(node["fastFindText"]),
		FastFindMatchColor:     semanticString(node["fastFindMatchColor"]),
		SelectedCount:          semanticInt(node["selectedCount"]),
		SelectedSize:           appInt64(node["selectedSize"]),
		TotalCount:             semanticInt(node["totalCount"]),
		TotalSize:              appInt64(node["totalSize"]),
	}
	if rawMatches, ok := node["fastFindMatches"].(map[string]any); ok {
		panel.FastFindMatches = make(map[string]extui.FastFindMatchModel, len(rawMatches))
		for entryID, rawMatch := range rawMatches {
			match := appMap(rawMatch)
			panel.FastFindMatches[entryID] = extui.FastFindMatchModel{
				Start:  semanticInt(match["start"]),
				Length: semanticInt(match["length"]),
			}
		}
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
	entry := extui.FileEntryModel{
		Index:            semanticInt(node["index"]),
		EntryID:          semanticString(node["entryId"]),
		Name:             semanticString(node["name"]),
		DisplayBaseName:  semanticString(node["displayBaseName"]),
		DisplayExtension: semanticString(node["displayExtension"]),
		Path:             semanticString(node["path"]),
		LocalPath:        semanticString(node["localPath"]),
		Size:             appInt64(node["size"]),
		SizeText:         semanticString(node["sizeText"]),
		IsDir:            appBool(node["isDir"]),
		IsUp:             appBool(node["isUp"]),
		IsHidden:         appBool(node["isHidden"]),
		IsExecutable:     appBool(node["isExecutable"]),
		IsImage:          appBool(node["isImage"]),
		Selected:         appBool(node["selected"]),
		SizeCalculated:   appBool(node["sizeCalculated"]),
		MTime:            semanticString(node["mtime"]),
		MTimeNanos:       appInt64(node["mtimeNanos"]),
		Version:          semanticString(node["version"]),
		Mode:             semanticString(node["mode"]),
		HighlightStyleID: semanticString(node["highlightStyleId"]),
	}
	if source := appMap(node["source"]); source != nil {
		entry.Source = &extui.ImageSourceModel{
			ResourceID:      semanticString(source["resourceId"]),
			SourceKey:       semanticString(source["sourceKey"]),
			Version:         semanticString(source["version"]),
			VersionStrength: semanticString(source["versionStrength"]),
			Size:            appInt64(source["size"]),
			SizeKnown:       appBool(source["sizeKnown"]),
			AccessProfile:   semanticString(source["accessProfile"]),
			StorageClass:    semanticString(source["storageClass"]),
		}
	}
	return entry
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
		IconKey:        semanticString(node["iconKey"]),
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
		CursorPosition:   semanticInt(node["cursorPosition"]),
		SelectionStart:   semanticInt(node["selectionStart"]),
		SelectionEnd:     semanticInt(node["selectionEnd"]),
		CursorVisible:    appBool(node["cursorVisible"]),
		CursorShape:      semanticString(node["cursorShape"]),
	}
}

func appTerminalFromLegacy(node map[string]any) extui.TerminalModel {
	term := extui.TerminalModel{
		ID:                semanticString(node["id"]),
		Title:             semanticString(node["title"]),
		DefaultBackground: semanticString(node["defaultBackground"]),
		Visible:           appBoolDefault(node["visible"], true),
		Focused:           appBool(node["focused"]),
		AltScreen:         appBool(node["altScreen"]),
		Busy:              appBool(node["busy"]),
		FollowTail: appBoolDefault(node["followTail"],
			appInt64(node["contentExtent"]) <= 0 ||
				appInt64(node["viewportStart"])+appInt64(node["viewportSpan"]) >=
					appInt64(node["contentExtent"])),
		CursorX:            semanticInt(node["cursorX"]),
		CursorY:            semanticInt(node["cursorY"]),
		CursorAbsoluteRow:  appInt64(node["cursorAbsoluteRow"]),
		CursorVisible:      appBool(node["cursorVisible"]),
		CursorShape:        semanticString(node["cursorShape"]),
		SelectionEnabled:   appBool(node["selectionEnabled"]),
		DocumentKey:        semanticString(node["documentKey"]),
		ScrollAction:       semanticString(node["scrollAction"]),
		ScrollUnit:         semanticString(node["scrollUnit"]),
		WindowStart:        appInt64(node["windowStart"]),
		WindowEnd:          appInt64(node["windowEnd"]),
		ViewportStart:      appInt64(node["viewportStart"]),
		ViewportSpan:       appInt64(node["viewportSpan"]),
		ContentExtent:      appInt64(node["contentExtent"]),
		ContentExtentKnown: appBool(node["contentExtentKnown"]),
		ViewportRow:        semanticInt(node["viewportRow"]),
		ViewportRows:       semanticInt(node["viewportRows"]),
		WindowGeneration:   uint64(appInt64(node["windowGeneration"])),
		WindowContentKey:   semanticString(node["windowContentKey"]),
	}
	for _, row := range appMapSlice(node["rows"]) {
		term.Rows = append(term.Rows, appTextRowFromLegacy(row))
	}
	for _, row := range appMapSlice(node["windowRows"]) {
		term.WindowRows = append(term.WindowRows, appTextRowFromLegacy(row))
	}
	return term
}

func appSurfaceFromLegacy(node map[string]any) extui.SurfaceModel {
	surface := extui.SurfaceModel{
		ID:                 semanticString(node["id"]),
		Kind:               semanticString(node["kind"]),
		DefaultBackground:  semanticString(node["defaultBackground"]),
		Title:              semanticString(node["title"]),
		Path:               semanticString(node["path"]),
		LocalPath:          semanticString(node["localPath"]),
		BaseName:           semanticString(node["baseName"]),
		Mode:               semanticString(node["mode"]),
		TopBarLeft:         semanticString(node["topBarLeft"]),
		TopBarRight:        semanticString(node["topBarRight"]),
		IconColor:          semanticString(node["iconColor"]),
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
		DocumentKey:        semanticString(node["documentKey"]),
		ScrollAction:       semanticString(node["scrollAction"]),
		ScrollUnit:         semanticString(node["scrollUnit"]),
		WindowStart:        appInt64(node["windowStart"]),
		WindowEnd:          appInt64(node["windowEnd"]),
		ViewportStart:      appInt64(node["viewportStart"]),
		ViewportSpan:       appInt64(node["viewportSpan"]),
		ContentExtent:      appInt64(node["contentExtent"]),
		ContentExtentKnown: appBool(node["contentExtentKnown"]),
		ViewportRow:        semanticInt(node["viewportRow"]),
		ViewportRows:       semanticInt(node["viewportRows"]),
		CursorAbsoluteRow:  appInt64(node["cursorAbsoluteRow"]),
		WindowGeneration:   uint64(appInt64(node["windowGeneration"])),
		WindowContentKey:   semanticString(node["windowContentKey"]),
		Selection:          appBool(node["selection"]),
		Autocomplete:       appMap(node["autocomplete"]),
	}
	for _, row := range appMapSlice(node["rows"]) {
		surface.Rows = append(surface.Rows, appTextRowFromLegacy(row))
	}
	for _, row := range appMapSlice(node["windowRows"]) {
		surface.WindowRows = append(surface.WindowRows, appTextRowFromLegacy(row))
	}
	return surface
}

func appOperationsQueueFromLegacy(node map[string]any) extui.OperationsQueueModel {
	queue := extui.OperationsQueueModel{
		ID:              semanticString(node["id"]),
		Title:           semanticString(node["title"]),
		Selected:        semanticInt(node["selected"]),
		SelectedTaskID:  semanticInt(node["selectedTaskId"]),
		Top:             semanticInt(node["top"]),
		WorkspaceIndex:  semanticInt(node["workspaceIndex"]),
		WorkspaceNumber: semanticInt(node["workspaceNumber"]),
		TabID:           semanticString(node["tabId"]),
		ActiveCount:     semanticInt(node["activeCount"]),
		QueuedCount:     semanticInt(node["queuedCount"]),
		RunningCount:    semanticInt(node["runningCount"]),
		CompletedCount:  semanticInt(node["completedCount"]),
		ErrorCount:      semanticInt(node["errorCount"]),
		CancelledCount:  semanticInt(node["cancelledCount"]),
		HasActive:       appBool(node["hasActive"]),
		CanClear:        appBool(node["canClear"]),
		CanClose:        appBool(node["canClose"]),
		CancelText:      semanticString(node["cancelText"]),
		ClearText:       semanticString(node["clearText"]),
		EmptyText:       semanticString(node["emptyText"]),
		DetailsText:     semanticString(node["detailsText"]),
	}
	for _, source := range appMapSlice(node["columns"]) {
		queue.Columns = append(queue.Columns, extui.OperationsQueueColumnModel{
			ID:        semanticString(source["id"]),
			Title:     semanticString(source["title"]),
			Width:     semanticInt(source["width"]),
			Alignment: semanticString(source["alignment"]),
		})
	}
	for _, source := range appMapSlice(node["items"]) {
		queue.Items = append(queue.Items, extui.OperationsQueueItemModel{
			ID:              semanticString(source["id"]),
			TaskID:          semanticInt(source["taskId"]),
			Index:           semanticInt(source["index"]),
			Type:            semanticString(source["type"]),
			Description:     semanticString(source["description"]),
			State:           semanticString(source["state"]),
			StateClass:      semanticString(source["stateClass"]),
			Action:          semanticString(source["action"]),
			CurrentFile:     semanticString(source["currentFile"]),
			DisplayText:     semanticString(source["displayText"]),
			CurrentProgress: semanticInt(source["currentProgress"]),
			Progress:        semanticInt(source["progress"]),
			TotalText:       semanticString(source["totalText"]),
			Elapsed:         semanticString(source["elapsed"]),
			ETA:             semanticString(source["eta"]),
			Speed:           semanticString(source["speed"]),
			Error:           semanticString(source["error"]),
			Cancellable:     appBool(source["cancellable"]),
			HasDetails:      appBool(source["hasDetails"]),
			Terminal:        appBool(source["terminal"]),
			Active:          appBool(source["active"]),
			CancelPrompt:    semanticString(source["cancelPrompt"]),
		})
	}
	return queue
}

func appTextRowFromLegacy(node map[string]any) extui.TextRowModel {
	return extui.TextRowModel{
		Index:       semanticInt(node["index"]),
		VisualRow:   semanticInt(node["visualRow"]),
		LogicalLine: semanticInt(node["logicalLine"]),
		Offset:      appInt64(node["offset"]),
		EndOffset:   appInt64(node["endOffset"]),
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
	if role == "menuBar" && !menu.Active {
		// The closed native menu bar only paints its top-level labels. Shipping
		// every submenu here made a panel layout shortcut resend the complete
		// command tree merely because a hidden view-mode checkmark changed.
		// Active menu-bar snapshots still retain all children so opening a menu
		// and previewing adjacent submenus remains authoritative and immediate.
		for index := range menu.Items {
			menu.Items[index].Items = nil
			if _, present := menu.Items[index].Legacy["items"]; present {
				legacy := semanticShallowMapCopy(menu.Items[index].Legacy)
				delete(legacy, "items")
				menu.Items[index].Legacy = legacy
			}
		}
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
		Icon:      semanticString(node["icon"]),
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
		modelItem := extui.KeyBarItemModel{
			Index: semanticInt(item["index"]),
			Key:   semanticString(item["key"]),
			Text:  semanticString(item["text"]),
			Icon:  semanticString(item["icon"]),
		}
		for _, alternative := range appMapSlice(item["alternatives"]) {
			modelItem.Alternatives = append(modelItem.Alternatives,
				extui.KeyBarAlternativeModel{
					Modifier: semanticString(alternative["modifier"]),
					Text:     semanticString(alternative["text"]),
					Icon:     semanticString(alternative["icon"]),
				})
		}
		keyBar.Items = append(keyBar.Items, modelItem)
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
