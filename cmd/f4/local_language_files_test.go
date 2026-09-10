package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

func TestInitLangHonorsUseLocalLanguageFiles(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "lang"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "lang", "ru.lng"), []byte("[Language]\nName=Русский\nCode=ru\n\n[Strings]\nLanguageSettings.Title=LOCAL LANGUAGE\n"), 0600); err != nil {
		t.Fatal(err)
	}

	oldCfg := AppConfig
	oldConfigDir := cachedF4ConfigDir
	oldUseSystemProfiles := cachedF4Portable
	t.Cleanup(func() {
		cachedF4ConfigDir = oldConfigDir
		cachedF4Portable = oldUseSystemProfiles
		AppConfig = oldCfg
		InitLang()
	})
	_ = GetF4ConfigDir()
	cachedF4ConfigDir = tempDir

	AppConfig.Language = "ru"
	AppConfig.FallbackLanguage = ""
	AppConfig.UseLocalLanguageFiles = false
	InitLang()
	if got := Msg("LanguageSettings.Title"); got == "LOCAL LANGUAGE" {
		t.Fatal("local language file was loaded while the option was disabled")
	}

	AppConfig.UseLocalLanguageFiles = true
	InitLang()
	if got := Msg("LanguageSettings.Title"); got != "LOCAL LANGUAGE" {
		t.Fatalf("local language file was not loaded when enabled: %q", got)
	}
}

func TestInitHelpSystemHonorsUseLocalLanguageFiles(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "help"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "lang"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "help", "zz-test.hlf"), []byte("@LocalTopic\nLocal help content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "lang", "zz-test.lng"), []byte("[Language]\nName=Test\nCode=zz-test\n\n[Strings]\nHelp.PanelNav=LOCAL HELP LANGUAGE\n"), 0600); err != nil {
		t.Fatal(err)
	}

	oldCfg := AppConfig
	oldConfigDir := cachedF4ConfigDir
	oldUseSystemProfiles := cachedF4Portable
	oldHelpEngine := vtui.GlobalHelpEngine
	oldHelpActionStrings := helpActionStrings
	t.Cleanup(func() {
		cachedF4ConfigDir = oldConfigDir
		cachedF4Portable = oldUseSystemProfiles
		AppConfig = oldCfg
		vtui.GlobalHelpEngine = oldHelpEngine
		helpActionStrings = oldHelpActionStrings
	})
	_ = GetF4ConfigDir()
	cachedF4ConfigDir = tempDir

	AppConfig.HelpLanguage = "zz-test"
	AppConfig.UseLocalLanguageFiles = false
	if got := loadHelpLangStrings("zz-test"); got != nil {
		t.Fatal("local help language strings were loaded while the option was disabled")
	}
	InitHelpSystem()
	if topic := vtui.GlobalHelpEngine.GetTopic("LocalTopic"); topic != nil {
		t.Fatal("local help file was loaded while the option was disabled")
	}

	AppConfig.UseLocalLanguageFiles = true
	if got := loadHelpLangStrings("zz-test"); got["Help.PanelNav"] != "LOCAL HELP LANGUAGE" {
		t.Fatalf("local help language strings were not loaded when enabled: %#v", got)
	}
	InitHelpSystem()
	if topic := vtui.GlobalHelpEngine.GetTopic("LocalTopic"); topic == nil {
		t.Fatal("local help file was not loaded when enabled")
	}
}

func TestInitHelpSystemLoadsLocalEnglishHelpWhenEnabled(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tempDir, "help"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "help", "en.hlf"), []byte("@LocalEnglishTopic\nLocal English help content\n"), 0600); err != nil {
		t.Fatal(err)
	}

	oldCfg := AppConfig
	oldConfigDir := cachedF4ConfigDir
	oldUseSystemProfiles := cachedF4Portable
	oldHelpEngine := vtui.GlobalHelpEngine
	t.Cleanup(func() {
		cachedF4ConfigDir = oldConfigDir
		cachedF4Portable = oldUseSystemProfiles
		AppConfig = oldCfg
		vtui.GlobalHelpEngine = oldHelpEngine
	})
	_ = GetF4ConfigDir()
	cachedF4ConfigDir = tempDir

	AppConfig.HelpLanguage = "en"
	AppConfig.UseLocalLanguageFiles = true
	InitHelpSystem()
	if topic := vtui.GlobalHelpEngine.GetTopic("LocalEnglishTopic"); topic == nil {
		t.Fatal("local English help file was not loaded when enabled")
	}
}
