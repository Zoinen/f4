package mediainfo

import (
	"github.com/unxed/f4/sdk/f4settings"
)

func (plugin *Plugin) settingsProvider() *f4settings.StructProvider[Settings] {
	category := "metadata"
	group := "MediaInfo"
	field := func(name, label, description string, kind f4settings.Kind) f4settings.Field {
		return f4settings.Scalar("mediainfo."+name, category, group, label, description, kind)
	}
	fields := []f4settings.Field{
		field("ShowInPluginMenu", "Show MediaInfo in Plugins menu", "Make the MediaInfo command visible in the plugin menu; other access routes remain available.", f4settings.Boolean),
		field("EnableQuickView", "MediaInfo Quick View", "Allow metadata previews for supported files in Quick View.", f4settings.Boolean),
		field("UseEditor", "Open reports in editor", "Open detailed generated reports in the text editor instead of the report window.", f4settings.Boolean),
		field("Prefix", "Command prefix", "Command-line prefix. Empty disables it. Must start with a letter and contain letters, digits, underscore or hyphen.", f4settings.String),
		field("Language", "Report language", "Language of report display labels; canonical template and macro field names are unchanged.", f4settings.ChoiceKind),
		field("Template", "Inform template", "Optional MediaInfo-style or supported Go report template. Validated before save; maximum 64 KiB.", f4settings.Multiline),
	}
	fields[4].ChoicePresentation = "dropdown"
	fields[4].Choices = f4settings.Choices("auto:Interface language", "en:English", "ru:Russian")
	return &f4settings.StructProvider[Settings]{Definition: f4settings.Catalog{ID: "mediainfo", Categories: []f4settings.Category{{ID: category, Label: f4settings.Text{English: "Metadata & reports"}}}, Fields: fields}, Read: func() (Settings, error) {
		plugin.mu.Lock()
		store := plugin.store
		plugin.mu.Unlock()
		if store == nil {
			return Settings{}, f4settings.Error("MediaInfo is not initialized")
		}
		return store.snapshot(), nil
	}, Validate: func(s Settings) error { return normalizeSettings(s).validate() }, Save: func(before, next Settings) error {
		next = normalizeSettings(next)
		plugin.mu.Lock()
		prefix := plugin.prefix
		store := plugin.store
		plugin.mu.Unlock()
		if prefix == nil || store == nil {
			return f4settings.Error("MediaInfo is not initialized")
		}
		if err := prefix.SetPrefix(next.Prefix); err != nil {
			return err
		}
		if err := store.save(next); err != nil {
			if rollbackErr := prefix.SetPrefix(before.Prefix); rollbackErr != nil {
				plugin.log("MediaInfo prefix rollback failed: %v", rollbackErr)
			}
			return err
		}
		return nil
	}}
}
