package app

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtui"
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
		t.Run(tc.name, func(t *testing.T) {
			topic := GenerateKeysHelpTopic(tc.name, "t", tc.areas, "")
			engine := vtui.NewHelpEngine(nil)
			engine.AddTopic(topic)
			view := vtui.NewHelpView(engine, tc.name)
			screen := vtui.NewSilentScreenBuf()
			screen.AllocBuf(180, 25)
			// Help wraps source lines at the visible width so zoom can reflow them.
			view.SetPosition(0, 0, dialog.GeneratedHelpLineWidth+3, 20)
			view.Show(screen)
			narrowRows := len(view.CurrentTopic().Lines)
			for lineNo, line := range view.CurrentTopic().Lines {
				if width := runewidth.StringWidth(line); width > dialog.GeneratedHelpLineWidth {
					t.Errorf("line %d is %d columns wide, want <= %d: %q", lineNo, width, dialog.GeneratedHelpLineWidth, line)
				}
			}
			view.SetPosition(0, 0, 173, 20)
			view.Show(screen)
			if wideRows := len(view.CurrentTopic().Lines); wideRows >= narrowRows {
				t.Fatalf("wider help did not reflow generated descriptions: narrow=%d wide=%d", narrowRows, wideRows)
			}
		})
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
