package app

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/keymap"
	"strings"
	"testing"
)

func TestGenerateKeysHelpTopic_Russian(t *testing.T) {
	old := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	defer func() { keymap.GlobalHotkeysMgr = old }()

	oldLang := config.App.Language
	defer func() {
		config.App.Language = oldLang
		initLang()
	}()
	config.App.Language = "ru"
	initLang()

	topic := GenerateKeysHelpTopic("PanelNav", "t", []string{"Shell"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "Открыть файл в просмотрщике") {
		t.Errorf("Expected Russian description in generated topic\n---\n%s", joined)
	}

	topic2 := GenerateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	if !strings.Contains(strings.Join(topic2.Lines, "\n"), "Сохранить файл") {
		t.Errorf("Expected Russian editor description in generated topic")
	}
}

func TestGenerateKeysHelpTopicsFitHelpWidth(t *testing.T) {
	old := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = old })

	oldLang := config.App.Language
	t.Cleanup(func() {
		config.App.Language = oldLang
		initLang()
	})
	config.App.Language = "ru"
	initLang()

	for _, tc := range []struct {
		name  string
		areas []string
	}{
		{name: "PanelNav", areas: []string{"Shell", "Terminal", "Common"}},
		{name: "ViewerEditor", areas: []string{"Editor", "Viewer", "Common"}},
	} {
		topic := GenerateKeysHelpTopic(tc.name, "t", tc.areas, "")
		for lineNo, line := range topic.Lines {
			if width := runewidth.StringWidth(line); width > dialog.GeneratedHelpLineWidth {
				t.Errorf("%s line %d is %d columns wide, want <= %d: %q", tc.name, lineNo, width, dialog.GeneratedHelpLineWidth, line)
			}
		}
	}
}

// The generated help must follow the *help* language even when it
// differs from the UI language: a Russian .hlf gets Russian action
// descriptions with an English UI.
func TestGenerateKeysHelpTopic_HelpLanguageOverridesUI(t *testing.T) {
	old := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	defer func() { keymap.GlobalHotkeysMgr = old }()

	oldLang := config.App.Language
	defer func() {
		config.App.Language = oldLang
		initLang()
	}()
	config.App.Language = "en"
	initLang()

	oldStrings := dialog.HelpActionStrings
	defer func() { dialog.HelpActionStrings = oldStrings }()
	dialog.HelpActionStrings = dialog.LoadHelpLangStrings("ru")
	if dialog.HelpActionStrings == nil {
		t.Fatal("Russian help strings not found")
	}

	topic := GenerateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "Сохранить файл") {
		t.Errorf("Expected Russian description with English UI\n---\n%s", joined)
	}
	if !strings.Contains(joined, "Редактор:") {
		t.Errorf("Expected Russian area header\n---\n%s", joined)
	}
}
