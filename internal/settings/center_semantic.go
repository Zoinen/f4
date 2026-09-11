package settings

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

// Settings paints its decorations directly in the terminal. Export those from
// the same owner as the controls, so every frontend receives the complete page.
func (c *settingsCenter) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	node := c.Window.SemanticNode(ctx)
	node["layout"] = "settings"
	children, _ := node["children"].([]map[string]any)
	for _, child := range children {
		if child["id"] == vtui.SemanticID(c.sidebar) {
			icons := make([]string, len(c.categories))
			for i, category := range c.categories {
				icons[i] = settingsCategoryIcon(category.ID)
			}
			child["itemIcons"] = icons
			break
		}
	}
	children = append(children,
		settingsCaption("settings-search-label", settingsText("Search", "Search:"), c.search.X1, c.Y1+1, c.sidebar.X2-c.sidebar.X1+1, 1),
		settingsCaption("settings-category-title", c.categoryLabel(c.category), c.page.X1, c.Y1+1, c.page.X2-c.page.X1+1, 1),
	)
	if strings.TrimSpace(c.query) != "" {
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
	}
	return node
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
	case "terminal":
		return "square-terminal"
	case "history":
		return "clock-3"
	case "associations":
		return "file-type"
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
	// Coordinates inside a scrollable group describe its complete, unscrolled
	// content. The frontend owns pixel layout; the core retains scroll/focus.
	node["scrollable"], node["scrollTop"], node["contentHeight"] = true, v.scroll, v.total
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
				width := v.X2 - v.X1 - 5
				if row.controlX > 0 {
					width = row.controlX - 1
				}
				label := settingsCaption(vtui.SemanticID(row.control)+"-label", row.field.Label.Resolve(config.App.Language, i18n.Msg), v.X1+2, v.Y1+row.y, width, len(row.label))
				label["wrapText"] = len(row.label) > 1
				label["dimmed"] = !row.match
				label["explainTarget"] = vtui.SemanticID(row.control)
				children = append(children, label)
			}
			if control := controls[vtui.SemanticID(row.control)]; control != nil {
				kind := semantic.String(control["kind"])
				control["fillWidth"] = row.controlWidth == 0 && (kind == "edit" || kind == "multiLineEdit" || kind == "button" || kind == "comboBox" || adaptiveChoices)
				if adaptiveChoices {
					control["title"] = row.field.Label.Resolve(config.App.Language, i18n.Msg)
					control["x"], control["y"] = v.X1+2, v.Y1+row.y
					control["h"] = max(1, row.controlY+row.controlHeight)
					control["w"] = max(1, v.X2-v.X1-5)
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
			"x": v.X1, "y": v.Y1 + box.top, "w": max(1, v.X2-v.X1-1), "h": box.bottom - box.top + 1,
			"children": children}
		groups = append(groups, group)
	}
	node["children"] = groups
	return node
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
	if !v.Group.HandleSemanticAction(action) {
		return false
	}
	v.notifyFocus()
	return true
}

func (h *settingsHelp) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	node := h.Group.SemanticNode(ctx)
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
