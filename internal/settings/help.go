package settings

import (
	"fmt"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func InstallHelp(language string) {
	phrase := func(s string) string { return (f4settings.Text{English: s}).Resolve(language, dialog.HelpMsg) }
	catalogs := []f4settings.Catalog{(coreSettingsProvider{}).Catalog(), newCoreRecordSettingsProvider().Catalog(), (aiSettingsProvider{}).Catalog(), (hotkeySettingsProvider{}).Catalog(), (settingsOperationsProvider{}).Catalog(), (pluginSettingsProvider{}).Catalog()}
	settingsProviders.RLock()
	providers := append([]f4settings.Provider(nil), settingsProviders.providers...)
	settingsProviders.RUnlock()
	for _, p := range providers {
		catalogs = append(catalogs, p.Catalog())
	}
	title := (f4settings.Text{English: "Settings", Key: "SettingsCenter.Title"}).Resolve(language, dialog.HelpMsg)
	index := &vtui.HelpTopic{Name: "SettingsCenter", StickyRows: 1, Lines: []string{title, "", phrase("Search dims unrelated options without hiding or disabling them."), phrase("Apply saves changes. OK saves and closes. Cancel discards later edits."), phrase("Select an option to read its explanation. Settings scroll independently."), ""}}
	for _, category := range Categories {
		index.Lines = append(index.Lines, category.Label.Resolve(language, dialog.HelpMsg))
		add := func(f f4settings.Field) {
			label := f.Label.Resolve(language, dialog.HelpMsg)
			description := f.Description.Resolve(language, dialog.HelpMsg)
			topic := &vtui.HelpTopic{Name: "Setting." + f.ID, StickyRows: 1, Lines: []string{label, ""}}
			topic.Lines = append(topic.Lines, vtui.WrapText(description, dialog.GeneratedHelpLineWidth)...)
			if f.Timing != "" {
				topic.Lines = append(topic.Lines, "", fmt.Sprintf(phrase("Takes effect: %s"), phrase(f.Timing)))
			}
			if f.Unavailable != "" {
				topic.Lines = append(topic.Lines, vtui.WrapText(fmt.Sprintf(phrase("Unavailable: %s"), phrase(f.Unavailable)), dialog.GeneratedHelpLineWidth)...)
			}
			vtui.GlobalHelpEngine.AddTopic(topic)
			index.Lines = append(index.Lines, "~"+label+"~Setting."+f.ID+"@")
		}
		for _, catalog := range catalogs {
			for _, field := range catalog.Fields {
				if field.Category == category.ID {
					add(field)
				}
			}
			for _, collection := range catalog.Collections {
				if collection.Category == category.ID {
					for _, field := range collection.Fields {
						add(field)
					}
				}
			}
		}
		index.Lines = append(index.Lines, "")
	}
	vtui.GlobalHelpEngine.AddTopic(index)
}
