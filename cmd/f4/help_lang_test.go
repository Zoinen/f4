package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestHelpLanguageSwitch(t *testing.T) {
	tempDir := t.TempDir()
	oldHelpEngine := vtui.GlobalHelpEngine
	oldHelpActionStrings := helpActionStrings
	t.Cleanup(func() {
		vtui.GlobalHelpEngine = oldHelpEngine
		helpActionStrings = oldHelpActionStrings
	})

	err := os.MkdirAll(filepath.Join(tempDir, "help"), 0700)
	if err != nil {
		t.Fatalf("failed to create help dir: %v", err)
	}

	ruHelpContent := "@TestTopic\n$RU Title\nRussian Help Content\n"
	err = os.WriteFile(filepath.Join(tempDir, "help", "ru.hlf"), []byte(ruHelpContent), 0600)
	if err != nil {
		t.Fatalf("failed to write test hlf: %v", err)
	}

	// Ensure GetF4ConfigDir's once-only detector cannot overwrite the test
	// override on its first call from InitHelpSystem.
	_ = GetF4ConfigDir()
	oldF4ConfigDir := cachedF4ConfigDir
	oldHelpLanguage := AppConfig.HelpLanguage
	cachedF4ConfigDir = tempDir
	t.Cleanup(func() {
		cachedF4ConfigDir = oldF4ConfigDir
		AppConfig.HelpLanguage = oldHelpLanguage
	})

	AppConfig.HelpLanguage = "ru"
	InitHelpSystem()

	topic := vtui.GlobalHelpEngine.GetTopic("TestTopic")
	if topic == nil {
		t.Errorf("expected TestTopic to be loaded from ru.hlf")
	} else if topic.Name != "TestTopic" || len(topic.Lines) == 0 || !strings.Contains(topic.Lines[0], "Russian Help Content") {
		t.Logf("Found TestTopic: %+v", topic)
	}
}

func TestHelpAndLangCompleteness(t *testing.T) {
	langs, err := filepath.Glob("lang/*.lng")
	if err != nil {
		t.Fatal(err)
	}
	helps, err := filepath.Glob("help/*.hlf")
	if err != nil {
		t.Fatal(err)
	}

	langSet := make(map[string]bool)
	for _, l := range langs {
		base := filepath.Base(l)
		langSet[strings.TrimSuffix(base, ".lng")] = true
	}

	helpSet := make(map[string]bool)
	for _, h := range helps {
		base := filepath.Base(h)
		helpSet[strings.TrimSuffix(base, ".hlf")] = true
	}

	for l := range langSet {
		if !helpSet[l] {
			t.Errorf("Language %q has a .lng file but is missing a corresponding .hlf help file.", l)
		}
	}

	for h := range helpSet {
		if !langSet[h] {
			t.Errorf("Language %q has a .hlf help file but is missing a corresponding .lng file.", h)
		}
	}
}
