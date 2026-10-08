package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

func TestTerminalHistoryTranslationsAndHelp(t *testing.T) {
	oldConfig, oldEngine, oldHelpStrings := config.App, vtui.GlobalHelpEngine, dialog.HelpActionStrings
	t.Cleanup(func() {
		config.App, vtui.GlobalHelpEngine, dialog.HelpActionStrings = oldConfig, oldEngine, oldHelpStrings
	})
	config.App.Language = "en"
	config.App.UseLocalLanguageFiles = false
	root := testutil.ModuleRootDir(t)
	paths, err := filepath.Glob(filepath.Join(root, "internal", "i18n", "lang", "*.lng"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("language inventory unavailable: %v", err)
	}
	for _, path := range paths {
		code := strings.TrimSuffix(filepath.Base(path), ".lng")
		t.Run(code, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			pack := ini.Parse(strings.NewReader(string(data)))
			label := pack.GetString("Strings", "SettingsCenter.InheritTerminalHistory.Label", "")
			description := pack.GetString("Strings", "SettingsCenter.InheritTerminalHistory.Description", "")
			if label == "" || description == "" {
				t.Fatal("option label or description is missing from this translation")
			}
			if code != "en" && (label == "Inherit terminal history in new tabs" || strings.HasPrefix(description, "Copy completed terminal output")) {
				t.Fatal("option still contains untranslated English")
			}
			help, err := dialog.EmbeddedHelp(code)
			if err != nil {
				t.Fatalf("embedded translated help is missing: %v", err)
			}
			vtui.GlobalHelpEngine = vtui.NewHelpEngine(dialog.NewMemoryHelpVFS(map[string]string{"history.hlf": help}))
			if err := vtui.GlobalHelpEngine.LoadFile("history.hlf"); err != nil {
				t.Fatal(err)
			}
			page := vtui.GlobalHelpEngine.GetTopic("TerminalHistoryInheritance")
			if page == nil || len(page.Lines) == 0 || page.Lines[0] != label {
				t.Fatal("translated terminal history help page is missing or has the wrong title")
			}
			body := strings.Join(page.Lines, "\n")
			if !strings.Contains(body, description) || !strings.Contains(body, "InheritTerminalHistory") || !strings.Contains(body, "settings.ini") {
				t.Fatal("help does not include the translated explanation and configuration key")
			}
			contents := vtui.GlobalHelpEngine.GetTopic("Contents")
			if contents == nil || !strings.Contains(strings.Join(contents.Lines, "\n"), "~"+label+"~TerminalHistoryInheritance@") {
				t.Fatal("translated help index does not link to the new page")
			}
			dialog.HelpActionStrings = dialog.LoadHelpLangStrings(code)
			InstallHelp(code)
			optionHelp := vtui.GlobalHelpEngine.GetTopic("Setting.InheritTerminalHistory")
			if optionHelp == nil || optionHelp.Lines[0] != label || !strings.Contains(strings.Join(optionHelp.Lines, "\n"), "~"+label+"~TerminalHistoryInheritance@") {
				t.Fatal("F1 option help is not translated or does not link to the detailed page")
			}
		})
	}
}
