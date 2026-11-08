package settings

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// Settings paints its decorations directly in the terminal. Export those from
// the same owner as the controls, so every frontend receives the complete page.
func (c *settingsCenter) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	node := c.Window.SemanticNode(ctx)
	node["layout"] = "settings"
	children, _ := node["children"].([]map[string]any)
	if c.recordOnly || c.fileGroupEditor {
		children = nil
		items := []vtui.UIElement{c.page, c.help, c.apply, c.ok, c.cancel}
		if c.fileGroupEditorSample != nil {
			items = append(items, c.fileGroupEditorSample)
		}
		for _, item := range items {
			for _, child := range node["children"].([]map[string]any) {
				if child["id"] == vtui.SemanticID(item) {
					children = append(children, child)
				}
			}
		}
	}
	for _, child := range children {
		if child["id"] == vtui.SemanticID(c.sidebar) {
			icons := make([]string, len(c.categories))
			states := make([]map[string]any, len(c.categories))
			searching := strings.TrimSpace(c.query) != ""
			for i, category := range c.categories {
				icons[i] = settingsCategoryIcon(category.ID)
				states[i] = map[string]any{"dimmed": searching && c.categoryMatches(category.ID) == 0}
			}
			child["itemIcons"] = icons
			child["itemStates"] = states
			break
		}
	}
	if !c.recordOnly && !c.fileGroupEditor {
		children = append(children,
			settingsCaption("settings-search-label", settingsText("Search", "Search:"), c.search.X1, c.Y1+1, c.sidebar.X2-c.sidebar.X1+1, 1),
			settingsCaption("settings-category-title", c.categoryLabel(c.category), c.page.X1, c.Y1+1, c.page.X2-c.page.X1+1, 1),
		)
	}
	if !c.fileGroupEditor && strings.TrimSpace(c.query) != "" {
		count := 0
		for _, category := range c.categories {
			count += c.categoryMatches(category.ID)
		}
		children = append(children, settingsCaption("settings-search-matches", fmt.Sprintf(settingsText("Matches", "Matches: %d"), count), c.sidebar.X1, c.Y1+3, c.sidebar.X2-c.sidebar.X1+1, 1))
		children[len(children)-2]["dimmed"] = c.categoryMatches(c.category) == 0
	}
	if c.status != "" {
		children = append(children, settingsCaption("settings-status", c.status, c.X1+2, c.apply.Y1, max(1, c.apply.X1-c.X1-3), 1))
	}
	node["children"] = children
	roles := map[string]string{
		vtui.SemanticID(c.search): "search", vtui.SemanticID(c.sidebar): "navigation",
		vtui.SemanticID(c.page): "content", vtui.SemanticID(c.help): "description",
		vtui.SemanticID(c.apply): "apply", vtui.SemanticID(c.ok): "accept", vtui.SemanticID(c.cancel): "cancel",
		vtui.SemanticID(c.clearSearch): "search-clear", vtui.SemanticID(c.previous): "search-previous",
		vtui.SemanticID(c.next): "search-next", "settings-search-label": "search-label",
		"settings-category-title": "content-title", "settings-search-matches": "search-matches",
		"settings-status": "footer-status",
	}
	for _, child := range children {
		child["layoutRole"] = roles[semantic.String(child["id"])]
		if c.fileGroupEditorSample != nil && child["id"] == vtui.SemanticID(c.fileGroupEditorSample) {
			child["layoutRole"] = "editor-preview"
		}
	}
	if c.fileGroupEditor {
		node["settingsSectionsOnly"] = true
		var mark func(map[string]any)
		mark = func(n map[string]any) {
			id := semantic.String(n["id"])
			if section := semantic.String(n["sectionID"]); section != "" {
				n["collapsible"] = true
				n["collapsed"] = c.fileGroupCollapsed[section]
			}
			field := strings.TrimPrefix(id, "id:setting:")
			if state := fileGroupColourState(field); state >= 0 && n["kind"] == "edit" {
				n["colorMatrixCell"] = true
				foreground, background := theme.FarColorComponents(semantic.String(n["text"]))
				n["colorComponents"] = map[string]string{"foreground": foreground, "background": background}
				n["items"] = fileGroupStateLabels()
				if c.fileGroupEditorSample != nil {
					patch := fileGroupColourPatches(c.fileGroupEditorSample.preview.style)[state]
					n["colorPreview"] = map[string]any{"foreground": patch.Foreground, "background": patch.Background}
				}
			}
			if header := semantic.AppMap(n["headerControl"]); len(header) > 0 {
				mark(header)
			}
			for _, child := range semantic.AppMapSlice(n["children"]) {
				mark(child)
			}
		}
		for _, child := range children {
			mark(child)
		}
	}
	return node
}

func (c *settingsCenter) HandleSemanticAction(action map[string]any) bool {
	if c.fileGroupEditor && action["action"] == "dialog.geometry" &&
		action["target"] == vtui.SemanticID(c) {
		w, h := semantic.Int(action["w"]), semantic.Int(action["h"])
		if w < 1 || h < 1 {
			return false
		}
		x, y := semantic.Int(action["x"]), semantic.Int(action["y"])
		c.SetPosition(x, y, x+w-1, y+h-1)
		return true
	}
	if c.fileGroupEditor && action["action"] == "control.toggleSection" {
		for _, box := range c.page.boxes {
			if box.section == "" || action["target"] != c.page.sectionID(box.section) {
				continue
			}
			if c.fileGroupCollapsed == nil {
				c.fileGroupCollapsed = make(map[string]bool)
			}
			c.fileGroupCollapsed[box.section] = !c.fileGroupCollapsed[box.section]
			c.rebuildCategory()
			return true
		}
		return false
	}
	if c.fileGroupEditor && semantic.String(action["action"]) == "control.reset" {
		id := strings.TrimPrefix(semantic.String(action["target"]), "id:setting:")
		if strings.HasPrefix(id, "group.") {
			c.resetFileGroupEditorField(id)
			return true
		}
		return false
	}
	return c.Window.HandleSemanticAction(action)
}

// Category IDs are stable across translations and search-result captions.
func settingsCategoryIcon(id string) string {
	switch id {
	case "appearance":
		return "palette"
	case "startup":
		return "circle-play"
	case "workspaces":
		return "panels-top-left"
	case "panels":
		return "columns-2"
	case "drives":
		return "hard-drive"
	case "operations":
		return "copy"
	case "editor":
		return "file-pen-line"
	case "syntax":
		return "file-code"
	case "keyboard":
		return "keyboard"
	case "hotkeys":
		return "key-round"
	case "terminal":
		return "square-terminal"
	case "history":
		return "clock-3"
	case "associations", "filegroups":
		return "file-type"
	case "filegroup-Conditions":
		return "list-filter"
	case "filegroup-Highlighting":
		return "palette"
	case "filegroup-Sorting":
		return "arrow-down-up"
	case "filegroup-Associations":
		return "square-terminal"
	case "menus":
		return "menu"
	case "network":
		return "network"
	case "metadata":
		return "file-text"
	case "updates":
		return "refresh-cw"
	case "plugins":
		return "plug"
	case "ai":
		return "sparkles"
	default:
		return "file-cog"
	}
}

func settingsCaption(id, text string, x, y, width, height int) map[string]any {
	return map[string]any{"id": id, "kind": "text", "text": text, "visible": true,
		"x": x, "y": y, "w": max(1, width), "h": max(1, height)}
}

func (v *settingsViewport) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	node := v.Group.SemanticNode(ctx)
	if _, ok := v.fullPage.(*fileGroupsOverview); ok {
		return v.fullPage.(interface {
			SemanticNode(*vtui.SemanticContext) map[string]any
		}).SemanticNode(ctx)
	}
	if page, ok := v.fullPage.(interface {
		SemanticNode(*vtui.SemanticContext) map[string]any
	}); ok {
		content := page.SemanticNode(ctx)
		node["children"] = content["children"]
		node["fullPage"] = true
		vtui.DebugLog("[FIX:settings-full-page] exported page=%s", vtui.SemanticID(v.fullPage))
		return node
	}
	// Coordinates inside a scrollable group describe its complete, unscrolled
	// content. The frontend owns pixel layout; the core retains scroll/focus.
	node["scrollable"], node["scrollTop"], node["contentHeight"] = true, v.scroll, v.total
	if v.scrollRequest.revision > 0 {
		node["scrollRequest"] = map[string]any{"revision": v.scrollRequest.revision, "top": v.scrollRequest.top}
	}
	controls := map[string]map[string]any{}
	children, _ := node["children"].([]map[string]any)
	for _, control := range children {
		settingsTranslateSemanticY(control, v.scroll)
		controls[semantic.String(control["id"])] = control
	}
	var groups []map[string]any
	for index, box := range v.boxes {
		matched := false
		var children []map[string]any
		for _, row := range box.rows {
			if row.heading {
				continue
			}
			matched = matched || row.match
			_, adaptiveChoices := row.control.(*settingsRadios)
			if len(row.label) > 0 && !adaptiveChoices {
				width := row.labelWidth
				if row.controlX > row.labelX {
					width = row.controlX - row.labelX - 1
				}
				label := settingsCaption(vtui.SemanticID(row.control)+"-label", row.field.Label.Resolve(config.App.Language, i18n.Msg), v.X1+2+row.labelX, v.Y1+row.y, width, len(row.label))
				label["wrapText"] = len(row.label) > 1
				label["dimmed"] = !row.match
				label["explainTarget"] = vtui.SemanticID(row.control)
				if row.field.ID == "group.Mask" && box.rows[0].conditionToggle && !row.control.IsDisabled() {
					label["focusTarget"] = vtui.SemanticID(row.control)
				}
				children = append(children, label)
			}
			if control := controls[vtui.SemanticID(row.control)]; control != nil {
				if row.field.IconTheme != "" && row.read != nil {
					control["iconTheme"] = row.field.IconTheme
					control["iconReference"] = row.read()
				}
				if row.field.ID == "group.Mask" && box.rows[0].conditionToggle {
					values := row.session.draft.Values
					control["maskChips"] = filegroups.MaskPreviewParts(values["group.Mask"], values["group.MaskSyntax"] == "legacy-glob")
				}
				kind := semantic.String(control["kind"])
				control["fillWidth"] = row.controlWidth == 0 && (kind == "edit" || kind == "multiLineEdit" || kind == "button" || kind == "comboBox" || adaptiveChoices)
				if row.field.Group == "Colors" && fileGroupColourState(row.field.ID) >= 0 && row.labelWidth < box.width-4 {
					control["colorMatrixColumnWidth"] = row.labelWidth
				}
				if adaptiveChoices {
					control["title"] = row.field.Label.Resolve(config.App.Language, i18n.Msg)
					control["x"], control["y"] = v.X1+2+row.labelX, v.Y1+row.y
					control["h"] = max(1, row.controlY+row.controlHeight)
					control["w"] = row.labelWidth
					if box.paired && row.field.Group == "File name matching" {
						// The three case options need more width than the two syntax options.
						extra := row.labelWidth / 5
						control["compactChoices"] = true
						control["fillWidth"] = false
						if row.field.ID == "group.Case" {
							control["x"] = v.X1 + 2 + row.labelX - extra
							control["w"] = row.labelWidth + extra
						} else {
							control["w"] = row.labelWidth - extra
						}
					}
				}
				control["dimmed"] = !row.match
				control["explainTarget"] = vtui.SemanticID(row.control)
				if _, ok := row.control.(*settingsCheckbox); ok {
					control["text"] = row.field.Label.Resolve(config.App.Language, i18n.Msg)
				}
				children = append(children, control)
			}
		}
		group := map[string]any{"id": fmt.Sprintf("%s-group-%d", vtui.SemanticID(v), index), "kind": "group",
			"title": box.title, "bordered": true, "visible": true, "dimmed": !matched,
			"x": v.X1 + box.left, "y": v.Y1 + box.top, "w": box.width, "h": box.bottom - box.top + 1,
			"children": children}
		if box.width < max(1, v.X2-v.X1-1) {
			group["conditionColumn"] = true
		}
		if box.section != "" {
			group["id"] = v.sectionID(box.section)
			group["sectionID"] = box.section
			group["layoutRole"] = "section-header"
			group["iconName"] = fileGroupSectionIcon(box.section)
			group["bordered"] = false
			group["dimmed"] = false
		}
		if box.rows[0].checkableHeader && !box.rows[0].sectionHeading {
			group["id"] = vtui.SemanticID(v) + "-" + box.rows[0].field.ID
			if box.rows[0].conditionToggle {
				group["layoutRole"] = "condition-group"

			}
		}
		for _, row := range box.rows {
			if row.checkableHeader {
				controls[vtui.SemanticID(row.control)]["text"] = box.title
				group["headerControl"] = controls[vtui.SemanticID(row.control)]
				// Keep the ordinary child for older semantic frontends. Qt docks
				// this same control into the title instead of rendering it twice.
				break
			}
		}
		groups = append(groups, group)
	}
	node["children"] = groups
	return node
}

func (v *settingsViewport) sectionID(section string) string {
	return vtui.SemanticID(v) + "-section-" + section
}

func fileGroupSectionIcon(section string) string {
	switch section {
	case "filegroup-Conditions":
		return "list-checks"
	case "filegroup-Highlighting":
		return "palette"
	case "filegroup-Icons":
		return "images"
	case "filegroup-Sorting":
		return "arrow-down-a-z"
	case "filegroup-Associations":
		return "plug"
	case "filegroup-Reset":
		return "rotate-ccw"
	}
	return ""
}

// Only mutate freshly exported maps; the live Go controls keep their terminal
// positions and owners, including the focus containers used for actions.
func settingsTranslateSemanticY(node map[string]any, dy int) {
	node["y"] = semantic.Int(node["y"]) + dy
	children, _ := node["children"].([]map[string]any)
	for _, child := range children {
		settingsTranslateSemanticY(child, dy)
	}
}

func (v *settingsViewport) HandleSemanticAction(action map[string]any) bool {
	// Hover is informational, including for unavailable controls. Do not route
	// it through focus navigation or ensureVisible, which would move the page.
	if semantic.String(action["action"]) == "control.explain" {
		for _, row := range v.rows {
			if row.heading || row.control == nil || semantic.String(action["target"]) != vtui.SemanticID(row.control) {
				continue
			}
			if radios, ok := row.control.(*settingsRadios); ok {
				previous := radios.hover
				defer func() { radios.hover = previous }()
				radios.hover = -1
				if value, exists := action["index"]; exists {
					index := semantic.Int(value)
					if index < 0 || index >= len(radios.buttons) {
						return false
					}
					radios.hover = index
				}
			}
			if v.onFocus != nil {
				v.onFocus(row)
			}
			return true
		}
		return false
	}
	if semantic.String(action["target"]) == vtui.SemanticID(v) && semantic.String(action["action"]) == "control.scroll" {
		v.scroll = max(0, min(semantic.Int(action["value"]), max(0, v.total-(v.Y2-v.Y1+1))))
		v.positionRows()
		vtui.DebugLog("[FIX] SETTINGS: semantic page scroll=%d total=%d", v.scroll, v.total)
		return true
	}
	focused := v.GetFocusedItem()
	wasFocused := v.IsFocused()
	if !v.Group.HandleSemanticAction(action) {
		return false
	}
	name := semantic.String(action["action"])
	if wasFocused && focused != nil && v.GetFocusedItem() == focused &&
		(name == "control.focus" || name == "focus") && semantic.String(action["target"]) == vtui.SemanticID(focused) {
		// Native focus acknowledgements can arrive after a touchpad scroll.
		// Reaffirming the same field must not reveal it over the user's scroll.
		vtui.DebugLog("[FIX:settings-scroll] same-target focus ack target=%s scroll=%d preserved", vtui.SemanticID(focused), v.scroll)
		return true
	}
	v.notifyFocus()
	return true
}

func (h *settingsHelp) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	node := h.Group.SemanticNode(ctx)
	paragraphs := strings.Split(h.text, "\n\n")
	if len(paragraphs) > 1 {
		node["title"] = paragraphs[0]
	}
	timingPrefix := fmt.Sprintf(Phrase("Takes effect: %s"), "")
	for _, paragraph := range paragraphs {
		if strings.HasPrefix(paragraph, timingPrefix) {
			node["helpTiming"] = paragraph
		}
	}
	height := len(settingsWrap(h.text, max(1, h.X2-h.X1)))
	node["scrollable"], node["scrollTop"], node["contentHeight"] = true, h.top, height
	text := settingsCaption(vtui.SemanticID(h)+"-text", h.text, h.X1, h.Y1, h.X2-h.X1, height)
	text["wrapText"] = true
	node["children"] = []map[string]any{text}
	return node
}

func (h *settingsHelp) HandleSemanticAction(action map[string]any) bool {
	if semantic.String(action["target"]) == vtui.SemanticID(h) && semantic.String(action["action"]) == "control.scroll" {
		height := len(settingsWrap(h.text, max(1, h.X2-h.X1)))
		h.top = max(0, min(semantic.Int(action["value"]), max(0, height-(h.Y2-h.Y1+1))))
		return true
	}
	return h.Group.HandleSemanticAction(action)
}
