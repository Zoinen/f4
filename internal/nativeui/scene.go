package nativeui

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"strings"
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
		Width:          semantic.Int(legacy["width"]),
		Height:         semantic.Int(legacy["height"]),
		ActiveScreen:   semantic.Int(legacy["activeScreen"]),
		WorkspaceCount: semantic.Int(legacy["workspaceCount"]),
		WorkspaceTabs:  appQueueAwareWorkspaceTabs(legacy),
		Presentation:   string(config.ParseGuiPresentationMode(string(config.App.GuiPresentation))),
		QmlIconSet:     string(config.ParseQmlIconSetMode(string(config.App.QmlIconSet))),
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

	if menu := semantic.AppMap(legacy["menuBar"]); menu != nil {
		m := appMenuFromLegacy(menu, "menuBar")
		scene.MenuBar = &m
	}
	if keyBar := semantic.AppMap(legacy["keyBar"]); keyBar != nil {
		k := appKeyBarFromLegacy(keyBar)
		scene.KeyBar = &k
	}
	if toast := semantic.AppMap(legacy["toast"]); toast != nil {
		scene.Toast = &extui.ToastModel{Message: semantic.String(toast["message"])}
	}

	for _, frame := range semantic.AppMapSlice(legacy["frames"]) {
		if autocompletes.isLegacyFrame(frame) || vmenus.isLegacyFrame(frame) {
			continue
		}
		switch semantic.String(frame["kind"]) {
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
	if scene.OperationsQueue == nil {
		scene.OperationsQueue = fileops.BackgroundOperationsQueue()
	}
	commitMediaPanelsForSemanticScene(legacy)
	return scene.ToMap()
}

func semanticMediaPanelIDs(legacy map[string]any) []string {
	seen := make(map[string]struct{})
	panelIDs := make([]string, 0, 4)
	collectFrames := func(frames []map[string]any) {
		for _, frame := range frames {
			kind := semantic.String(frame["kind"])
			if kind != "panels" && kind != "shell" {
				continue
			}
			for _, panel := range semantic.AppMapSlice(frame["panels"]) {
				panelID := semantic.String(panel["id"])
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
		for _, screen := range semantic.AppMapSlice(screens) {
			collectFrames(semantic.AppMapSlice(screen["frames"]))
		}
	}
	if len(panelIDs) == 0 {
		collectFrames(semantic.AppMapSlice(legacy["frames"]))
	}
	return panelIDs
}

func commitMediaPanelsForSemanticScene(legacy map[string]any) {
	if broker := plughost.CurrentExtUiMediaBroker(); broker != nil {
		broker.CommitScenePanels(semanticMediaPanelIDs(legacy))
	}
}

func appQueueAwareWorkspaceTabs(legacy map[string]any) map[string]any {
	source := semantic.AppMap(legacy["workspaceTabs"])
	if source == nil {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	tabs := semantic.AppMapSlice(source["tabs"])
	clonedTabs := make([]map[string]any, 0, len(tabs))
	for _, tab := range tabs {
		clone := make(map[string]any, len(tab))
		for key, value := range tab {
			clone[key] = value
		}
		clonedTabs = append(clonedTabs, clone)
	}

	for screenIndex, screen := range semantic.AppMapSlice(legacy["screens"]) {
		queueCanClose := true
		surfaceKind, iconName := appWorkspaceTabPresentation(screen)
		if surfaceKind == "operationsQueue" {
			for _, frame := range semantic.AppMapSlice(screen["frames"]) {
				if semantic.String(frame["kind"]) == "operationsQueue" {
					queueCanClose = semantic.AppBool(frame["canClose"])
					break
				}
			}
		}
		screenNumber := semantic.Int(screen["number"])
		for _, tab := range clonedTabs {
			if semantic.Int(tab["index"]) == screenIndex ||
				(screenNumber > 0 && semantic.Int(tab["number"]) == screenNumber) {
				if surfaceKind != "" {
					tab["surfaceKind"] = surfaceKind
					tab["iconName"] = iconName
					if _, structuredMarker := tab["marker"]; structuredMarker {
						// Current semantic scenes already split the frame kind
						// into marker. Preserve legitimate titles such as
						// "P projects" instead of treating their first letter
						// as an old cell-renderer prefix.
						tab["text"] = strings.TrimSpace(semantic.String(tab["text"]))
					} else {
						tab["text"] = appWorkspaceTabText(semantic.String(tab["text"]))
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

func appWorkspaceTabPresentation(screen map[string]any) (kind, iconName string) {
	frames := semantic.AppMapSlice(screen["frames"])
	for index := len(frames) - 1; index >= 0; index-- {
		frame := frames[index]
		switch semantic.String(frame["kind"]) {
		case "operationsQueue":
			return "operationsQueue", "list-checks"
		case "editor":
			return "editor", "file-pen-line"
		case "viewer":
			return "viewer", "file-text"
		case "terminal":
			return "terminal", "square-terminal"
		case "panels", "shell":
			terminalActive := semantic.AppBool(frame["terminalActive"])
			if showPanels, present := frame["showPanels"]; present && !semantic.AppBool(showPanels) {
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
	for _, tab := range semantic.AppMapSlice(workspaceTabs["tabs"]) {
		if semantic.Int(tab["index"]) != queue.WorkspaceIndex && !semantic.AppBool(tab["active"]) {
			continue
		}
		queue.TabID = semantic.String(tab["id"])
		queue.WorkspaceNumber = semantic.Int(tab["number"])
		return
	}
}

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
	if semantic.String(node["kind"]) == "widget" {
		if element := elements[semantic.String(node["id"])]; element != nil {
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
	for _, child := range semantic.AppMapSlice(node["children"]) {
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
		menu, bottomHint := FrameVMenu(frame)
		if menu != nil {
			out = append(out, appVMenu{
				frame:          frame,
				menu:           menu,
				bottomHint:     bottomHint,
				menuBarSubmenu: IsMenuBarSubmenu(frame, menuBar),
			})
		}
	}
	return out
}

func IsMenuBarSubmenu(frame vtui.Frame, menuBar *vtui.MenuBar) bool {
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

func FrameVMenu(frame vtui.Frame) (*vtui.VMenu, string) {
	switch item := frame.(type) {
	case *vtui.VMenu:
		return item, ""
	case interface{ SemanticMenuControl() (*vtui.VMenu, string) }:
		return item.SemanticMenuControl()
	case interface{ MenuControl() *vtui.VMenu }:
		return item.MenuControl(), ""
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
	if owner, ok := item.menu.GetOwner().(*vtui.ComboBox); ok && owner.DropdownOnly {
		menu.OwnerID = vtui.SemanticID(owner)
		menu.Presentation = "dropdown"
	}
	if parent := item.menu.ParentFrame(); parent != nil {
		menu.ParentID = vtui.SemanticID(parent)
		menu.AnchorIndex = item.menu.ParentIndex()
	} else if parent := item.menu.ParentMenu(); parent != nil {
		// Compatibility for menu controls linked before their owning frame
		// identity was recorded.
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
			Details:    source.Details,
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
			HasSubmenu: item.menu.HasSubmenu(i),
		})
	}
	return menu
}

func (menus appVMenus) isLegacyFrame(frame map[string]any) bool {
	id := semantic.String(frame["id"])
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
	frames := semantic.AppMapSlice(legacy["frames"])
	filtered := make([]map[string]any, 0, len(frames))
	for _, frame := range frames {
		if !menus.isLegacyFrame(frame) {
			filtered = append(filtered, frame)
		}
	}
	out["frames"] = filtered
	return out
}

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
		kind := semantic.String(frame["kind"])
		if kind != "panels" && kind != "shell" {
			return frame
		}
		copyFrame := make(map[string]any, len(frame))
		for key, value := range frame {
			if key == "commandLine" {
				continue
			}
			if key == "panels" {
				panels := semantic.AppMapSlice(value)
				lightweight := make([]map[string]any, 0, len(panels))
				for _, panel := range panels {
					if !semantic.AppBool(panel["metadataDeferred"]) {
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
		frames := semantic.AppMapSlice(value)
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
	if screens := semantic.AppMapSlice(legacy["screens"]); len(screens) > 0 {
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
	kind := semantic.String(frame["kind"])
	if kind != "dialog" && kind != "window" {
		return false
	}
	id := semantic.String(frame["id"])
	x := semantic.Int(frame["x"])
	y := semantic.Int(frame["y"])
	w := semantic.Int(frame["w"])
	h := semantic.Int(frame["h"])
	for _, item := range menus {
		if id != "" && (id == item.id || id == item.windowID) {
			return true
		}
		if x == item.x && y == item.y && w == item.w && h == item.h && semantic.String(frame["title"]) == "" {
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
	layout := semantic.AppMap(node["panelLayout"])
	shell := extui.ShellModel{
		ID:             semantic.String(node["id"]),
		Title:          semantic.String(node["title"]),
		Mode:           "panels",
		ActivePanel:    semantic.Int(node["activePanel"]),
		ShowPanels:     semantic.AppBoolDefault(node["showPanels"], true),
		ShowLeftPanel:  semantic.AppBoolDefault(node["showLeftPanel"], true),
		ShowRightPanel: semantic.AppBoolDefault(node["showRightPanel"], true),
		Wide:           semantic.AppBool(node["wide"]),
		WidePanel:      semantic.Int(node["widePanel"]),
		PanelLayout: extui.PanelLayoutModel{
			Columns:              semantic.Int(layout["columns"]),
			SplitColumn:          semantic.Int(layout["splitColumn"]),
			LeftBottomInsetRows:  semantic.Int(layout["leftBottomInsetRows"]),
			RightBottomInsetRows: semantic.Int(layout["rightBottomInsetRows"]),
		},
		ShowKeyBar:     semantic.AppBoolDefault(node["showKeyBar"], true),
		TerminalBusy:   semantic.AppBool(node["terminalBusy"]),
		TerminalActive: semantic.AppBool(node["terminalActive"]),
		MacroRecording: semantic.AppBool(node["macroRecording"]),
		Fallback:       semantic.AppBool(node["fallback"]),
		FallbackReason: semantic.String(node["reason"]),
	}
	if shell.TerminalActive {
		shell.Mode = "terminal"
	}
	for _, panel := range semantic.AppMapSlice(node["panels"]) {
		shell.Panels = append(shell.Panels, appPanelFromLegacy(panel))
	}
	for _, panel := range semantic.AppMapSlice(node["infoPanels"]) {
		shell.InfoPanels = append(shell.InfoPanels, appInfoPanelFromLegacy(panel))
	}
	for _, panel := range semantic.AppMapSlice(node["quickViews"]) {
		shell.QuickViews = append(shell.QuickViews, appQuickViewFromLegacy(panel))
	}
	if cmd := semantic.AppMap(node["commandLine"]); cmd != nil {
		c := appCommandLineFromLegacy(cmd)
		shell.CommandLine = &c
	}
	if term := semantic.AppMap(node["terminal"]); term != nil {
		t := appTerminalFromLegacy(term)
		shell.Terminal = &t
	}
	return shell
}

func appInfoPanelFromLegacy(node map[string]any) extui.InfoPanelModel {
	panel := extui.InfoPanelModel{
		ID:         semantic.String(node["id"]),
		Side:       semantic.Int(node["side"]),
		Active:     semantic.AppBool(node["active"]),
		Title:      semantic.String(node["title"]),
		BottomHint: semantic.String(node["bottomHint"]),
	}
	for _, row := range semantic.AppMapSlice(node["rows"]) {
		panel.Rows = append(panel.Rows, extui.InfoPanelRowModel{
			Kind:  semantic.String(row["kind"]),
			Label: semantic.String(row["label"]),
			Value: semantic.String(row["value"]),
		})
	}
	return panel
}

func appQuickViewFromLegacy(node map[string]any) extui.QuickViewModel {
	view := extui.QuickViewModel{
		ID:          semantic.String(node["id"]),
		Side:        semantic.Int(node["side"]),
		SourceSide:  semantic.Int(node["sourceSide"]),
		Active:      semantic.AppBool(node["active"]),
		Title:       semantic.String(node["title"]),
		BottomHint:  semantic.String(node["bottomHint"]),
		ContentKey:  semantic.String(node["contentKey"]),
		Name:        semantic.String(node["name"]),
		Path:        semantic.String(node["path"]),
		SizeText:    semantic.String(node["sizeText"]),
		PreviewKind: semantic.String(node["previewKind"]),
		Label:       semantic.String(node["label"]),
		Error:       semantic.String(node["error"]),
		Loading:     semantic.AppBool(node["loading"]),
		Wrap:        semantic.AppBool(node["wrap"]),
		ImageSource: semantic.String(node["imageSource"]),
		ImageWidth:  semantic.Int(node["imageWidth"]),
		ImageHeight: semantic.Int(node["imageHeight"]),
	}
	for _, row := range semantic.AppMapSlice(node["headerRows"]) {
		view.HeaderRows = append(view.HeaderRows, appTextRowFromLegacy(row))
	}
	if surface := semantic.AppMap(node["surface"]); surface != nil {
		view.Surface = appSurfaceFromLegacy(surface)
	}
	return view
}

func appPanelFromLegacy(node map[string]any) extui.PanelModel {
	sourceKind := semantic.String(node["sourceKind"])
	if sourceKind == "" {
		sourceKind = "vfs"
	}
	galleryLayoutMode := semantic.String(node["galleryLayoutMode"])
	if _, ok := panel.ParseGalleryLayoutMode(galleryLayoutMode); !ok {
		galleryLayoutMode = string(panel.GalleryLayoutMasonry)
	}
	galleryColumnCount := semantic.Int(node["galleryColumnCount"])
	if galleryColumnCount < panel.MinGalleryColumnCount || galleryColumnCount > panel.MaxGalleryColumnCount {
		galleryColumnCount = panel.DefaultGalleryColumnCount
	}
	parsedGalleryLayoutMode, _ := panel.ParseGalleryLayoutMode(galleryLayoutMode)
	galleryDensity := semantic.Int(node["galleryDensity"])
	if galleryDensity > 0 {
		galleryDensity = panel.ClampGalleryDensity(parsedGalleryLayoutMode, galleryDensity)
	} else {
		galleryDensity, _, _ = panel.GalleryDensityLimits(parsedGalleryLayoutMode)
	}
	galleryDensities := make(map[string]int, len(panel.GalleryLayoutModes))
	suppliedGalleryDensities := semantic.AppMap(node["galleryDensities"])
	for _, mode := range panel.GalleryLayoutModes {
		density := semantic.Int(suppliedGalleryDensities[string(mode)])
		if density <= 0 {
			density, _, _ = panel.GalleryDensityLimits(mode)
			if density <= 0 {
				// The untouched compact default is derived from exact frontend
				// font metrics, so it remains absent until the user zooms.
				continue
			}
		}
		galleryDensities[string(mode)] = panel.ClampGalleryDensity(mode, density)
	}
	if galleryDensity > 0 {
		galleryDensities[string(parsedGalleryLayoutMode)] = galleryDensity
	}
	galleryLayoutRevision := semantic.AppInt64(node["galleryLayoutRevision"])
	if galleryLayoutRevision < 1 {
		galleryLayoutRevision = 1
	}
	panel := extui.PanelModel{
		PathIcon:            semantic.String(node["pathIcon"]),
		UseSortGroups:       semantic.AppBool(node["useSortGroups"]),
		SelectedFiles:       semantic.Int(node["selectedFiles"]),
		SelectedDirectories: semantic.Int(node["selectedDirectories"]),
		FreeSpace:           uint64(semantic.AppInt64(node["freeSpace"])),
		FreeSpaceKnown:      semantic.AppBool(node["freeSpaceKnown"]),
		SymlinkTarget:       semantic.String(node["symlinkTarget"]),

		ID:                     semantic.String(node["id"]),
		Side:                   semantic.Int(node["side"]),
		Active:                 semantic.AppBool(node["active"]),
		Path:                   semantic.String(node["path"]),
		Title:                  semantic.String(node["title"]),
		ShowFileInfo:           semantic.AppBool(node["showFileInfo"]),
		GalleryLayoutMode:      galleryLayoutMode,
		GalleryColumnCount:     galleryColumnCount,
		GalleryDensity:         galleryDensity,
		GalleryDensities:       galleryDensities,
		GalleryLayoutRevision:  galleryLayoutRevision,
		SourceKind:             sourceKind,
		DropAllowed:            semantic.AppBool(node["dropAllowed"]),
		PreviewCapable:         semantic.AppBool(node["previewCapable"]),
		CatalogRevision:        semantic.AppInt64(node["catalogRevision"]),
		SelectionRevision:      semantic.AppInt64(node["selectionRevision"]),
		MetadataDeferred:       semantic.AppBool(node["metadataDeferred"]),
		MetadataRevision:       semantic.AppInt64(node["metadataRevision"]),
		HighlightRevision:      semantic.AppInt64(node["highlightRevision"]),
		CursorEntryID:          semantic.String(node["cursorEntryId"]),
		SortMode:               semantic.String(node["sortModeName"]),
		SortReverse:            semantic.AppBool(node["sortReverse"]),
		SeparateFileExtensions: semantic.AppBool(node["separateFileExtensions"]),
		Cursor:                 semantic.Int(node["cursor"]),
		Loading:                semantic.AppBool(node["loading"]),
		CatalogProvisional:     semantic.AppBool(node["catalogProvisional"]),
		FastFind:               semantic.AppBool(node["fastFind"]),
		FastFindText:           semantic.String(node["fastFindText"]),
		FastFindMatchColor:     semantic.String(node["fastFindMatchColor"]),
		SelectedCount:          semantic.Int(node["selectedCount"]),
		SelectedSize:           semantic.AppInt64(node["selectedSize"]),
		TotalCount:             semantic.Int(node["totalCount"]),
		TotalSize:              semantic.AppInt64(node["totalSize"]),
	}
	if rawMatches, ok := node["fastFindMatches"].(map[string]any); ok {
		panel.FastFindMatches = make(map[string]extui.FastFindMatchModel, len(rawMatches))
		for entryID, rawMatch := range rawMatches {
			match := semantic.AppMap(rawMatch)
			panel.FastFindMatches[entryID] = extui.FastFindMatchModel{
				Start:  semantic.Int(match["start"]),
				Length: semantic.Int(match["length"]),
			}
		}
	}
	parseColumns := func(value any) []extui.PanelColumnModel {
		columns := semantic.AppMapSlice(value)
		result := make([]extui.PanelColumnModel, 0, len(columns))
		for _, column := range columns {
			result = append(result, extui.PanelColumnModel{
				ID:        semantic.String(column["id"]),
				Role:      semantic.String(column["role"]),
				Index:     semantic.Int(column["index"]),
				Title:     semantic.String(column["title"]),
				Width:     semantic.Int(column["width"]),
				Alignment: semantic.String(column["alignment"]),
				SortMode:  semantic.String(column["sortMode"]),
				Sortable:  semantic.AppBool(column["sortable"]),
			})
		}
		return result
	}
	panel.GalleryColumns = parseColumns(node["galleryColumns"])
	if rawStyles, ok := node["highlightStyles"].(map[string]any); ok {
		panel.HighlightStyles = make(map[string]extui.HighlightStyleModel, len(rawStyles))
		for id, rawStyle := range rawStyles {
			panel.HighlightStyles[id] = appHighlightStyleFromLegacy(semantic.AppMap(rawStyle))
		}
	}
	for _, entry := range semantic.AppMapSlice(node["entries"]) {
		panel.Entries = append(panel.Entries, appEntryFromLegacy(entry))
	}
	return panel
}

func appEntryFromLegacy(node map[string]any) extui.FileEntryModel {
	entry := extui.FileEntryModel{
		Index:            semantic.Int(node["index"]),
		EntryID:          semantic.String(node["entryId"]),
		Name:             semantic.String(node["name"]),
		DisplayBaseName:  semantic.String(node["displayBaseName"]),
		DisplayExtension: semantic.String(node["displayExtension"]),
		Path:             semantic.String(node["path"]),
		LocalPath:        semantic.String(node["localPath"]),
		Size:             semantic.AppInt64(node["size"]),
		SizeText:         semantic.String(node["sizeText"]),
		IsDir:            semantic.AppBool(node["isDir"]),
		IsUp:             semantic.AppBool(node["isUp"]),
		IsHidden:         semantic.AppBool(node["isHidden"]),
		IsExecutable:     semantic.AppBool(node["isExecutable"]),
		IsImage:          semantic.AppBool(node["isImage"]),
		Selected:         semantic.AppBool(node["selected"]),
		SizeCalculated:   semantic.AppBool(node["sizeCalculated"]),
		MTime:            semantic.String(node["mtime"]),
		MTimeNanos:       semantic.AppInt64(node["mtimeNanos"]),
		Version:          semantic.String(node["version"]),
		Mode:             semantic.String(node["mode"]),
		HighlightStyleID: semantic.String(node["highlightStyleId"]),
	}
	if source := semantic.AppMap(node["source"]); source != nil {
		entry.Source = &extui.ImageSourceModel{
			ResourceID:      semantic.String(source["resourceId"]),
			SourceKey:       semantic.String(source["sourceKey"]),
			Version:         semantic.String(source["version"]),
			VersionStrength: semantic.String(source["versionStrength"]),
			Size:            semantic.AppInt64(source["size"]),
			SizeKnown:       semantic.AppBool(source["sizeKnown"]),
			AccessProfile:   semantic.String(source["accessProfile"]),
			StorageClass:    semantic.String(source["storageClass"]),
		}
	}
	return entry
}

func appHighlightStyleFromLegacy(node map[string]any) extui.HighlightStyleModel {
	patch := func(value any) extui.HighlightColorPatchModel {
		m := semantic.AppMap(value)
		return extui.HighlightColorPatchModel{
			Foreground: semantic.String(m["foreground"]),
			Background: semantic.String(m["background"]),
		}
	}
	style := extui.HighlightStyleModel{
		Marker:         semantic.String(node["marker"]),
		IconKey:        semantic.String(node["iconKey"]),
		Icon:           semantic.String(node["icon"]),
		Normal:         patch(node["normal"]),
		Selected:       patch(node["selected"]),
		Cursor:         patch(node["cursor"]),
		SelectedCursor: patch(node["selectedCursor"]),
	}
	for _, group := range semantic.AppMapSlice(node["groups"]) {
		style.Groups = append(style.Groups, extui.HighlightGroupModel{
			ID:   semantic.String(group["id"]),
			Name: semantic.String(group["name"]),
		})
	}
	return style
}

func appCommandLineFromLegacy(node map[string]any) extui.CommandLineModel {
	return extui.CommandLineModel{
		ID:               semantic.String(node["id"]),
		Visible:          semantic.AppBoolDefault(node["visible"], true),
		Focused:          semantic.AppBool(node["focused"]),
		Prompt:           semantic.String(node["prompt"]),
		PromptRuns:       appRunsFromLegacy(node["promptRuns"]),
		Text:             semantic.String(node["text"]),
		Empty:            semantic.AppBool(node["empty"]),
		Runs:             appRunsFromLegacy(node["runs"]),
		InputX:           semantic.Int(node["inputX"]),
		CursorPrefixRuns: appRunsFromLegacy(node["cursorPrefixRuns"]),
		CursorX:          semantic.Int(node["cursorX"]),
		CursorPosition:   semantic.Int(node["cursorPosition"]),
		SelectionStart:   semantic.Int(node["selectionStart"]),
		SelectionEnd:     semantic.Int(node["selectionEnd"]),
		CursorVisible:    semantic.AppBool(node["cursorVisible"]),
		CursorShape:      semantic.String(node["cursorShape"]),
	}
}

func appTerminalFromLegacy(node map[string]any) extui.TerminalModel {
	term := extui.TerminalModel{
		ID:                semantic.String(node["id"]),
		Title:             semantic.String(node["title"]),
		DefaultBackground: semantic.String(node["defaultBackground"]),
		Visible:           semantic.AppBoolDefault(node["visible"], true),
		Focused:           semantic.AppBool(node["focused"]),
		AltScreen:         semantic.AppBool(node["altScreen"]),
		Busy:              semantic.AppBool(node["busy"]),
		FollowTail: semantic.AppBoolDefault(node["followTail"],
			semantic.AppInt64(node["contentExtent"]) <= 0 ||
				semantic.AppInt64(node["viewportStart"])+semantic.AppInt64(node["viewportSpan"]) >=
					semantic.AppInt64(node["contentExtent"])),
		CursorX:            semantic.Int(node["cursorX"]),
		CursorY:            semantic.Int(node["cursorY"]),
		CursorAbsoluteRow:  semantic.AppInt64(node["cursorAbsoluteRow"]),
		CursorVisible:      semantic.AppBool(node["cursorVisible"]),
		CursorShape:        semantic.String(node["cursorShape"]),
		SelectionEnabled:   semantic.AppBool(node["selectionEnabled"]),
		DocumentKey:        semantic.String(node["documentKey"]),
		ScrollAction:       semantic.String(node["scrollAction"]),
		ScrollUnit:         semantic.String(node["scrollUnit"]),
		WindowStart:        semantic.AppInt64(node["windowStart"]),
		WindowEnd:          semantic.AppInt64(node["windowEnd"]),
		ViewportStart:      semantic.AppInt64(node["viewportStart"]),
		ViewportSpan:       semantic.AppInt64(node["viewportSpan"]),
		ContentExtent:      semantic.AppInt64(node["contentExtent"]),
		ContentExtentKnown: semantic.AppBool(node["contentExtentKnown"]),
		ViewportRow:        semantic.Int(node["viewportRow"]),
		ViewportRows:       semantic.Int(node["viewportRows"]),
		WindowGeneration:   uint64(semantic.AppInt64(node["windowGeneration"])),
		WindowContentKey:   semantic.String(node["windowContentKey"]),
	}
	for _, row := range semantic.AppMapSlice(node["rows"]) {
		term.Rows = append(term.Rows, appTextRowFromLegacy(row))
	}
	for _, row := range semantic.AppMapSlice(node["windowRows"]) {
		term.WindowRows = append(term.WindowRows, appTextRowFromLegacy(row))
	}
	return term
}

func appSurfaceFromLegacy(node map[string]any) extui.SurfaceModel {
	surface := extui.SurfaceModel{
		ID:                      semantic.String(node["id"]),
		Kind:                    semantic.String(node["kind"]),
		DefaultBackground:       semantic.String(node["defaultBackground"]),
		Title:                   semantic.String(node["title"]),
		Path:                    semantic.String(node["path"]),
		LocalPath:               semantic.String(node["localPath"]),
		BaseName:                semantic.String(node["baseName"]),
		Mode:                    semantic.String(node["mode"]),
		TopBarLeft:              semantic.String(node["topBarLeft"]),
		TopBarRight:             semantic.String(node["topBarRight"]),
		IconColor:               semantic.String(node["iconColor"]),
		Busy:                    semantic.AppBool(node["busy"]),
		Dirty:                   semantic.AppBool(node["dirty"]),
		Saving:                  semantic.AppBool(node["saving"]),
		HexMode:                 semantic.AppBool(node["hexMode"]),
		WrapMode:                semantic.AppBool(node["wrapMode"]),
		WordWrap:                semantic.AppBool(node["wordWrap"]),
		Overtype:                semantic.AppBool(node["overtype"]),
		TopOffset:               semantic.AppInt64(node["topOffset"]),
		Size:                    semantic.AppInt64(node["size"]),
		CursorLine:              semantic.Int(node["cursorLine"]),
		CursorPos:               semantic.Int(node["cursorPos"]),
		CursorVisualRow:         semantic.Int(node["cursorVisualRow"]),
		CursorVisualColumn:      semantic.Int(node["cursorVisualColumn"]),
		CursorVisible:           semantic.AppBool(node["cursorVisible"]),
		CursorShape:             semantic.String(node["cursorShape"]),
		CursorAbsoluteColumn:    semantic.Int(node["cursorAbsoluteColumn"]),
		ScrollTop:               semantic.Int(node["scrollTop"]),
		ScrollLeft:              semantic.Int(node["scrollLeft"]),
		DocumentKey:             semantic.String(node["documentKey"]),
		ScrollAction:            semantic.String(node["scrollAction"]),
		ScrollUnit:              semantic.String(node["scrollUnit"]),
		WindowStart:             semantic.AppInt64(node["windowStart"]),
		WindowEnd:               semantic.AppInt64(node["windowEnd"]),
		ViewportStart:           semantic.AppInt64(node["viewportStart"]),
		ViewportSpan:            semantic.AppInt64(node["viewportSpan"]),
		ContentExtent:           semantic.AppInt64(node["contentExtent"]),
		ContentExtentKnown:      semantic.AppBool(node["contentExtentKnown"]),
		ViewportRow:             semantic.Int(node["viewportRow"]),
		ViewportRows:            semantic.Int(node["viewportRows"]),
		CursorAbsoluteRow:       semantic.AppInt64(node["cursorAbsoluteRow"]),
		WindowGeneration:        uint64(semantic.AppInt64(node["windowGeneration"])),
		ViewportColumns:         semantic.Int(node["viewportColumns"]),
		WindowRequestGeneration: uint64(semantic.AppInt64(node["windowRequestGeneration"])),
		GeometryRevision:        uint64(semantic.AppInt64(node["geometryRevision"])),
		LayoutRevision:          uint64(semantic.AppInt64(node["layoutRevision"])),
		LayoutPending:           semantic.Bool(node["layoutPending"]),
		LoadError:               semantic.String(node["loadError"]),
		WindowContentKey:        semantic.String(node["windowContentKey"]),
		Selection:               semantic.AppBool(node["selection"]),
		SelectionAnchorRow:      semantic.AppInt64(node["selectionAnchorRow"]),
		SelectionAnchorColumn:   semantic.Int(node["selectionAnchorColumn"]),
		SelectionForeground:     semantic.String(node["selectionForeground"]),
		SelectionBackground:     semantic.String(node["selectionBackground"]),
		SelectionBold:           semantic.AppBool(node["selectionBold"]),
		SelectionUnderline:      semantic.AppBool(node["selectionUnderline"]),
		SelectionStrikeout:      semantic.AppBool(node["selectionStrikeout"]),
		Autocomplete:            semantic.AppMap(node["autocomplete"]),
	}
	for _, row := range semantic.AppMapSlice(node["rows"]) {
		surface.Rows = append(surface.Rows, appTextRowFromLegacy(row))
	}
	for _, row := range semantic.AppMapSlice(node["windowRows"]) {
		surface.WindowRows = append(surface.WindowRows, appTextRowFromLegacy(row))
	}
	for _, caret := range semantic.AppMapSlice(node["secondaryCarets"]) {
		surface.SecondaryCarets = append(surface.SecondaryCarets, extui.CaretModel{
			CursorAbsoluteRow:     semantic.Int64(caret["cursorAbsoluteRow"]),
			CursorAbsoluteColumn:  semantic.Int(caret["cursorAbsoluteColumn"]),
			Selection:             semantic.AppBool(caret["selection"]),
			SelectionAnchorRow:    semantic.Int64(caret["selectionAnchorRow"]),
			SelectionAnchorColumn: semantic.Int(caret["selectionAnchorColumn"]),
		})
	}

	return surface
}

func appOperationsQueueFromLegacy(node map[string]any) extui.OperationsQueueModel {
	queue := extui.OperationsQueueModel{
		ID:              semantic.String(node["id"]),
		Title:           semantic.String(node["title"]),
		Selected:        semantic.Int(node["selected"]),
		SelectedTaskID:  semantic.Int(node["selectedTaskId"]),
		Top:             semantic.Int(node["top"]),
		WorkspaceIndex:  semantic.Int(node["workspaceIndex"]),
		WorkspaceNumber: semantic.Int(node["workspaceNumber"]),
		TabID:           semantic.String(node["tabId"]),
		ActiveCount:     semantic.Int(node["activeCount"]),
		QueuedCount:     semantic.Int(node["queuedCount"]),
		RunningCount:    semantic.Int(node["runningCount"]),
		CompletedCount:  semantic.Int(node["completedCount"]),
		ErrorCount:      semantic.Int(node["errorCount"]),
		CancelledCount:  semantic.Int(node["cancelledCount"]),
		HasActive:       semantic.AppBool(node["hasActive"]),
		CanClear:        semantic.AppBool(node["canClear"]),
		CanClose:        semantic.AppBool(node["canClose"]),
		CancelText:      semantic.String(node["cancelText"]),
		ClearText:       semantic.String(node["clearText"]),
		EmptyText:       semantic.String(node["emptyText"]),
		DetailsText:     semantic.String(node["detailsText"]),
	}
	for _, source := range semantic.AppMapSlice(node["columns"]) {
		queue.Columns = append(queue.Columns, extui.OperationsQueueColumnModel{
			ID:        semantic.String(source["id"]),
			Title:     semantic.String(source["title"]),
			Width:     semantic.Int(source["width"]),
			Alignment: semantic.String(source["alignment"]),
		})
	}
	for _, source := range semantic.AppMapSlice(node["items"]) {
		queue.Items = append(queue.Items, extui.OperationsQueueItemModel{
			ID:              semantic.String(source["id"]),
			TaskID:          semantic.Int(source["taskId"]),
			Index:           semantic.Int(source["index"]),
			Type:            semantic.String(source["type"]),
			Description:     semantic.String(source["description"]),
			State:           semantic.String(source["state"]),
			StateClass:      semantic.String(source["stateClass"]),
			Action:          semantic.String(source["action"]),
			CurrentFile:     semantic.String(source["currentFile"]),
			DisplayText:     semantic.String(source["displayText"]),
			CurrentProgress: semantic.Int(source["currentProgress"]),
			Progress:        semantic.Int(source["progress"]),
			TotalText:       semantic.String(source["totalText"]),
			Elapsed:         semantic.String(source["elapsed"]),
			ETA:             semantic.String(source["eta"]),
			Speed:           semantic.String(source["speed"]),
			Error:           semantic.String(source["error"]),
			Cancellable:     semantic.AppBool(source["cancellable"]),
			HasDetails:      semantic.AppBool(source["hasDetails"]),
			Terminal:        semantic.AppBool(source["terminal"]),
			Active:          semantic.AppBool(source["active"]),
			CancelPrompt:    semantic.String(source["cancelPrompt"]),
		})
	}
	return queue
}

func appTextRowFromLegacy(node map[string]any) extui.TextRowModel {
	return extui.TextRowModel{
		Index:          semantic.Int(node["index"]),
		VisualRow:      semantic.Int(node["visualRow"]),
		LogicalLine:    semantic.Int(node["logicalLine"]),
		Offset:         semantic.AppInt64(node["offset"]),
		EndOffset:      semantic.AppInt64(node["endOffset"]),
		VisualWidth:    semantic.Int(node["visualWidth"]),
		HasVisualWidth: node["visualWidth"] != nil,
		Text:           semantic.String(node["text"]),
		Runs:           appRunsFromLegacy(node["runs"]),
	}
}

func appMenuFromLegacy(node map[string]any, role string) extui.MenuModel {
	menu := extui.MenuModel{
		ID:       semantic.String(node["id"]),
		Role:     role,
		Title:    semantic.String(node["title"]),
		Active:   semantic.AppBool(node["active"]),
		Selected: semantic.Int(node["selected"]),
		Legacy:   node,
	}
	for _, item := range semantic.AppMapSlice(node["items"]) {
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
				legacy := semantic.SemanticShallowMapCopy(menu.Items[index].Legacy)
				delete(legacy, "items")
				menu.Items[index].Legacy = legacy
			}
		}
	}
	return menu
}

func appMenuItemFromLegacy(node map[string]any) extui.MenuItemModel {
	text, checked := appNormalizeMenuCheckmark(semantic.String(node["text"]))
	item := extui.MenuItemModel{
		Index:     semantic.Int(node["index"]),
		Text:      text,
		RawText:   semantic.String(node["rawText"]),
		Hotkey:    semantic.String(node["hotkey"]),
		Icon:      semantic.String(node["icon"]),
		Shortcut:  semantic.String(node["shortcut"]),
		Command:   semantic.Int(node["command"]),
		Separator: semantic.AppBool(node["separator"]),
		Disabled:  semantic.AppBool(node["disabled"]),
		Checked:   checked || semantic.AppBool(node["checked"]),
		Legacy:    node,
	}
	switch details := node["details"].(type) {
	case map[string]string:
		item.Details = details
	case map[string]any:
		item.Details = make(map[string]string, len(details))
		for key, value := range details {
			item.Details[key] = semantic.String(value)
		}
	}

	for _, child := range semantic.AppMapSlice(node["items"]) {
		item.Items = append(item.Items, appMenuItemFromLegacy(child))
	}
	return item
}

func appKeyBarFromLegacy(node map[string]any) extui.KeyBarModel {
	keyBar := extui.KeyBarModel{
		ID:       semantic.String(node["id"]),
		Visible:  semantic.AppBoolDefault(node["visible"], true),
		Modifier: semantic.String(node["modifier"]),
	}
	for _, item := range semantic.AppMapSlice(node["items"]) {
		modelItem := extui.KeyBarItemModel{
			Index: semantic.Int(item["index"]),
			Key:   semantic.String(item["key"]),
			Text:  semantic.String(item["text"]),
			Icon:  semantic.String(item["icon"]),
		}
		for _, alternative := range semantic.AppMapSlice(item["alternatives"]) {
			modelItem.Alternatives = append(modelItem.Alternatives,
				extui.KeyBarAlternativeModel{
					Modifier: semantic.String(alternative["modifier"]),
					Text:     semantic.String(alternative["text"]),
					Icon:     semantic.String(alternative["icon"]),
				})
		}
		keyBar.Items = append(keyBar.Items, modelItem)
	}
	return keyBar
}

func appDialogFromLegacy(node map[string]any) extui.DialogModel {
	dlg := extui.DialogModel{
		ID:        semantic.String(node["id"]),
		Kind:      semantic.String(node["kind"]),
		Title:     semantic.String(node["title"]),
		Modal:     semantic.AppBool(node["modal"]),
		Busy:      semantic.AppBool(node["busy"]),
		Progress:  semantic.Int(node["progress"]),
		ShowClose: semantic.AppBool(node["showClose"]),
		Legacy:    node,
	}
	for _, child := range semantic.AppMapSlice(node["children"]) {
		dlg.Controls = append(dlg.Controls, appControlFromLegacy(child))
	}
	return dlg
}

func appControlFromLegacy(node map[string]any) extui.ControlModel {
	ctrl := extui.ControlModel{
		WrapText:   semantic.AppBool(node["wrapText"]),
		ID:         semantic.String(node["id"]),
		Kind:       semantic.String(node["kind"]),
		Visible:    semantic.AppBoolDefault(node["visible"], true),
		Focused:    semantic.AppBool(node["focused"]),
		Disabled:   semantic.AppBool(node["disabled"]),
		Text:       semantic.String(node["text"]),
		Title:      semantic.String(node["title"]),
		Hotkey:     semantic.String(node["hotkey"]),
		State:      semantic.Int(node["state"]),
		ThreeState: semantic.AppBool(node["threeState"]),
		Default:    semantic.AppBool(node["default"]),
		Password:   semantic.AppBool(node["password"]),
		Cursor:     semantic.Int(node["cursor"]),
		Left:       semantic.Int(node["left"]),
		Selected:   semantic.AppIntSlice(node["selected"]),
		Items:      semantic.AppStringSlice(node["items"]),
		Rows:       semantic.AppMapSlice(node["rows"]),
		Legacy:     node,
	}
	for _, child := range semantic.AppMapSlice(node["children"]) {
		ctrl.Children = append(ctrl.Children, appControlFromLegacy(child))
	}
	return ctrl
}

func appRunsFromLegacy(value any) []extui.RunModel {
	var runs []extui.RunModel
	for _, run := range semantic.AppMapSlice(value) {
		runs = append(runs, extui.RunModel{
			Text:       semantic.String(run["text"]),
			Attr:       uint64(semantic.AppInt64(run["attr"])),
			Foreground: semantic.String(run["foreground"]),
			Background: semantic.String(run["background"]),
			Bold:       semantic.AppBool(run["bold"]),
			Underline:  semantic.AppBool(run["underline"]),
			Strikeout:  semantic.AppBool(run["strikeout"]),
		})
	}
	return runs
}
