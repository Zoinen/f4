package main

import (
	"strings"
	"testing"
)

func TestGenerateKeysHelpTopic_Turkish(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	oldLang := AppConfig.Language
	defer func() {
		AppConfig.Language = oldLang
		InitLang()
	}()
	AppConfig.Language = "tr"
	InitLang()

	topic := generateKeysHelpTopic("PanelNav", "t", []string{"Shell"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "Dosyayı görüntüleyicide aç") {
		t.Errorf("Expected Turkish description in generated topic\n---\n%s", joined)
	}

	topic2 := generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	if !strings.Contains(strings.Join(topic2.Lines, "\n"), "Dosyayı kaydet") {
		t.Errorf("Expected Turkish editor description in generated topic")
	}
}

func TestGenerateKeysHelpTopic_HelpLanguageOverridesUI_Turkish(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	oldLang := AppConfig.Language
	defer func() {
		AppConfig.Language = oldLang
		InitLang()
	}()
	AppConfig.Language = "en"
	InitLang()

	oldStrings := helpActionStrings
	defer func() { helpActionStrings = oldStrings }()
	helpActionStrings = loadHelpLangStrings("tr")
	if helpActionStrings == nil {
		t.Fatal("Turkish help strings not found")
	}

	topic := generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	joined := strings.Join(topic.Lines, "\n")
	if !strings.Contains(joined, "Dosyayı kaydet") {
		t.Errorf("Expected Turkish description with English UI\n---\n%s", joined)
	}
	if !strings.Contains(joined, "Düzenleyici:") {
		t.Errorf("Expected Turkish area header\n---\n%s", joined)
	}
}
