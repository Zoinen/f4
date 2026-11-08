package settings

import (
	"strings"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func isFileGroupAttributeField(id string) bool {
	switch strings.TrimPrefix(fileGroupFieldKey(id), "Highlight.") {
	case "IncludeAttributes", "ExcludeAttributes":
		return true
	}
	return false
}

func newFileGroupAttributeCombo(value string, change func(string)) *vtui.ComboBox {
	combo := vtui.NewComboBox(0, 0, 20, nil)
	combo.DropdownOnly = true
	names := filegroups.AttributeNames()
	selected := make(map[string]bool, len(names))
	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		name := filegroups.CanonicalAttributeName(token)
		if name == "" {
			name = token
			if !selected[name] {
				names = append(names, name)
			}
		}
		selected[name] = true
	}
	rowText := func(name string) string {
		mark := "☐ "
		if selected[name] {
			mark = "☑ "
		}
		return mark + strings.ReplaceAll(name, "&", "&&")
	}
	for _, name := range names {
		combo.Menu.AddItem(vtui.MenuItem{Text: rowText(name), KeepOpen: true})
	}
	updateSummary := func(raw string) {
		if raw == "" {
			raw = Phrase("None")
		}
		combo.Edit.SetText(raw)
	}
	updateSummary(value)
	combo.Menu.OnAction = func(i int) {
		name := names[i]
		selected[name] = !selected[name]
		combo.Menu.Items[i].Text = rowText(name)
		values := make([]string, 0, len(names))
		for _, name := range names {
			if selected[name] {
				values = append(values, name)
			}
		}
		raw := strings.Join(values, ", ")
		updateSummary(raw)
		change(raw)
	}
	combo.Menu.OnKeyDown = func(event *vtinput.InputEvent) bool {
		if event.VirtualKeyCode != vtinput.VK_SPACE {
			return false
		}
		activate := *event
		activate.VirtualKeyCode = vtinput.VK_RETURN
		return combo.Menu.ProcessKey(&activate)
	}
	return combo
}
