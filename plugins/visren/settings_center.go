package visren

import (
	"github.com/unxed/f4/sdk/f4settings"
)

func settingsProvider() *f4settings.StructProvider[config] {
	f := f4settings.Scalar("visren.EditorFormat", "operations", "Visual File Renamer", "Rename editor format", "Choose original/target columns or target-only lines when opening the rename list in the editor.", f4settings.ChoiceKind)
	f.Choices = f4settings.Choices("source_target:Original and target", "targets_only:Target only")
	return &f4settings.StructProvider[config]{Definition: f4settings.Catalog{ID: "visren", Categories: []f4settings.Category{{ID: "operations", Label: f4settings.Text{English: "File operations"}}}, Fields: []f4settings.Field{f, f4settings.Scalar("visren.WordDiv", "operations", "Visual File Renamer", "Word delimiters", "Characters that split filename words for title/case transformations. Maximum 18 characters.", f4settings.String)}}, Read: func() (config, error) { return loadConfig(), nil }, Validate: func(c config) error {
		if len([]rune(c.WordDiv)) == 0 || len([]rune(c.WordDiv)) > 18 {
			return f4settings.Error("enter 1-18 word delimiters")
		}
		return nil
	}, Save: func(_, next config) error { return saveConfig(next) }}
}
