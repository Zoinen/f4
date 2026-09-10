package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/keymap"
	"strings"
	"testing"
)

func TestGenerateKeysHelpTopic_Turkish(t *testing.T) {
	old := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	defer func() { keymap.GlobalHotkeysMgr = old }()

	oldLang := config.App.Language
	defer func() {
		config.App.Language = oldLang
		initLang()
	}()
	config.App.Language = "tr"
	initLang()

	topic := GenerateKeysHelpTopic("PanelNav", "t", []string{"Shell"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "Dosyayı görüntüleyicide aç") {
		t.Errorf("Expected Turkish description in generated topic\n---\n%s", joined)
	}

	topic2 := GenerateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	if !strings.Contains(strings.Join(topic2.Lines, "\n"), "Dosyayı kaydet") {
		t.Errorf("Expected Turkish editor description in generated topic")
	}
}

func TestGenerateKeysHelpTopic_HelpLanguageOverridesUI_Turkish(t *testing.T) {
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
	dialog.HelpActionStrings = dialog.LoadHelpLangStrings("tr")
	if dialog.HelpActionStrings == nil {
		t.Fatal("Turkish help strings not found")
	}

	topic := GenerateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "Dosyayı kaydet") {
		t.Errorf("Expected Turkish description with English UI\n---\n%s", joined)
	}
	if !strings.Contains(joined, "Düzenleyici:") {
		t.Errorf("Expected Turkish area header\n---\n%s", joined)
	}
}
