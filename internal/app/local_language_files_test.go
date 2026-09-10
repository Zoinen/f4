package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/i18n"
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

	oldCfg := config.App
	oldConfigDir := config.CachedF4ConfigDir
	oldUseSystemProfiles := config.CachedF4Portable
	t.Cleanup(func() {
		config.CachedF4ConfigDir = oldConfigDir
		config.CachedF4Portable = oldUseSystemProfiles
		config.App = oldCfg
		initLang()
	})
	_ = config.GetF4ConfigDir()
	config.CachedF4ConfigDir = tempDir

	config.App.Language = "ru"
	config.App.FallbackLanguage = ""
	config.App.UseLocalLanguageFiles = false
	initLang()
	if got := i18n.Msg("LanguageSettings.Title"); got == "LOCAL LANGUAGE" {
		t.Fatal("local language file was loaded while the option was disabled")
	}

	config.App.UseLocalLanguageFiles = true
	initLang()
	if got := i18n.Msg("LanguageSettings.Title"); got != "LOCAL LANGUAGE" {
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

	oldCfg := config.App
	oldConfigDir := config.CachedF4ConfigDir
	oldUseSystemProfiles := config.CachedF4Portable
	oldHelpEngine := vtui.GlobalHelpEngine
	oldHelpActionStrings := dialog.HelpActionStrings
	t.Cleanup(func() {
		config.CachedF4ConfigDir = oldConfigDir
		config.CachedF4Portable = oldUseSystemProfiles
		config.App = oldCfg
		vtui.GlobalHelpEngine = oldHelpEngine
		dialog.HelpActionStrings = oldHelpActionStrings
	})
	_ = config.GetF4ConfigDir()
	config.CachedF4ConfigDir = tempDir

	config.App.HelpLanguage = "zz-test"
	config.App.UseLocalLanguageFiles = false
	if got := dialog.LoadHelpLangStrings("zz-test"); got != nil {
		t.Fatal("local help language strings were loaded while the option was disabled")
	}
	InitHelpSystem()
	if topic := vtui.GlobalHelpEngine.GetTopic("LocalTopic"); topic != nil {
		t.Fatal("local help file was loaded while the option was disabled")
	}

	config.App.UseLocalLanguageFiles = true
	if got := dialog.LoadHelpLangStrings("zz-test"); got["Help.PanelNav"] != "LOCAL HELP LANGUAGE" {
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

	oldCfg := config.App
	oldConfigDir := config.CachedF4ConfigDir
	oldUseSystemProfiles := config.CachedF4Portable
	oldHelpEngine := vtui.GlobalHelpEngine
	t.Cleanup(func() {
		config.CachedF4ConfigDir = oldConfigDir
		config.CachedF4Portable = oldUseSystemProfiles
		config.App = oldCfg
		vtui.GlobalHelpEngine = oldHelpEngine
	})
	_ = config.GetF4ConfigDir()
	config.CachedF4ConfigDir = tempDir

	config.App.HelpLanguage = "en"
	config.App.UseLocalLanguageFiles = true
	InitHelpSystem()
	if topic := vtui.GlobalHelpEngine.GetTopic("LocalEnglishTopic"); topic == nil {
		t.Fatal("local English help file was not loaded when enabled")
	}
}
