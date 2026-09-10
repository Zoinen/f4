package settings

import (
	_ "embed"
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
	"strings"
)

//go:embed choice_help.tsv
var settingsChoiceHelpData string

var settingsChoiceDescriptions = func() map[string]f4settings.Text {
	result := map[string]f4settings.Text{}
	for _, line := range strings.Split(strings.TrimSpace(settingsChoiceHelpData), "\n") {
		parts := strings.SplitN(strings.TrimSuffix(line, "\r"), "|", 3)
		if len(parts) == 3 {
			result[parts[0]+"|"+parts[1]] = f4settings.Text{English: parts[2]}
		}
	}
	return result
}()

func settingsFieldChoiceHelp(f f4settings.Field) f4settings.Field {
	f.Choices = append([]f4settings.Choice(nil), f.Choices...)
	for i := range f.Choices {
		choice := &f.Choices[i]
		if choice.Description.English != "" || choice.Description.Key != "" || len(choice.Description.Translations) != 0 {
			continue
		}
		id := f.ID
		if id == "netfox.ProxyMode" && choice.Value != "0" {
			id = "ProxyMode"
		}
		if strings.HasPrefix(id, "HistoryShowTimes.") {
			id = "HistoryShowTimes"
		}
		if strings.HasPrefix(id, "cloudfox.") && (strings.HasSuffix(id, ".storage") || strings.HasSuffix(id, ".credentials")) {
			id = "cloudfox." + id[strings.LastIndex(id, ".")+1:]
		}
		choice.Description = settingsChoiceDescriptions[id+"|"+choice.Value]
		if choice.Description.English == "" {
			choice.Description = f.Description
		}
	}
	return f
}

func settingsCatalogChoiceHelp(catalog f4settings.Catalog) f4settings.Catalog {
	catalog.Fields = append([]f4settings.Field(nil), catalog.Fields...)
	for i := range catalog.Fields {
		catalog.Fields[i] = settingsFieldChoiceHelp(catalog.Fields[i])
	}
	catalog.Collections = append([]f4settings.Collection(nil), catalog.Collections...)
	for i := range catalog.Collections {
		col := &catalog.Collections[i]
		col.Fields = append([]f4settings.Field(nil), col.Fields...)
		for j := range col.Fields {
			col.Fields[j] = settingsFieldChoiceHelp(col.Fields[j])
		}
	}
	return catalog
}

// The frame stack repaints the Center below an open dropdown. Reading the
// highlighted row here covers keyboard, type-ahead and mouse hover equally,
// without calling a provider or committing a choice while browsing.
func (c *settingsCenter) refreshChoiceHelp() {
	if vtui.FrameManager != nil {
		for _, row := range c.page.rows {
			combo, ok := row.control.(*vtui.ComboBox)
			if !ok || vtui.FrameManager.GetTopFrame() != combo.Menu {
				continue
			}
			// Keep the explanation visible, including below the content at
			// terminal widths where there is no separate right-hand pane.
			menu := combo.Menu
			width := min(menu.X2-menu.X1+1, c.page.X2-c.page.X1+1)
			height := min(menu.Y2-menu.Y1+1, c.page.Y2-c.page.Y1+1)
			x := max(c.page.X1, min(menu.X1, c.page.X2-width+1))
			y := max(c.page.Y1, min(menu.Y1, c.page.Y2-height+1))
			menu.SetPosition(x, y, x+width-1, y+height-1)
			index := combo.Menu.SelectPos
			if index >= 0 && index < len(row.field.Choices) {
				c.choiceHelpRow = row
				choice := row.field.Choices[index]
				description := choice.Description
				if description.English == "" && description.Key == "" && len(description.Translations) == 0 {
					description = row.field.Description
				}
				text := row.field.Label.Resolve(config.App.Language, i18n.Msg) + " — " + choice.Label.Resolve(config.App.Language, i18n.Msg) + "\n\n" + description.Resolve(config.App.Language, i18n.Msg)
				if row.field.Timing != "" {
					text += "\n\n" + fmt.Sprintf(Phrase("Takes effect: %s"), Phrase(row.field.Timing))
				}
				if text != c.help.text {
					c.help.text = text
					c.help.top = 0
				}
			}
			return
		}
	}
	if c.choiceHelpRow != nil {
		row := c.choiceHelpRow
		c.choiceHelpRow = nil
		for _, current := range c.page.rows {
			if current == row {
				c.describe(row)
				break
			}
		}
	}
}
