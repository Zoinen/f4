package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
	"reflect"
	"testing"
)

func TestPluginCommandRegistrationVisibilityAndCleanup(t *testing.T) {
	api := &CoreAPI{}
	visible := false
	registration, err := api.RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.plugin-command",
		Location: vfs.PluginCommandPanel,
		Label:    "Test command",
		Visible:  func(vfs.App) bool { return visible },
		Run:      func(vfs.App) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	if commands := plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, nil); len(commands) != 0 {
		t.Fatalf("hidden command was returned: %#v", commands)
	}
	visible = true
	commands := plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, nil)
	if len(commands) != 1 || commands[0].ID != "test.plugin-command" {
		t.Fatalf("visible command snapshot = %#v", commands)
	}
	if config := plughost.PluginCommandsSnapshot(vfs.PluginCommandConfig, nil); len(config) != 0 {
		t.Fatalf("command leaked into config menu: %#v", config)
	}

	registration.Unregister()
	registration.Unregister()
	if commands := plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, nil); len(commands) != 0 {
		t.Fatalf("unregistered command remains: %#v", commands)
	}
}

func TestPluginCommandRegistrationRejectsDuplicateID(t *testing.T) {
	api := &CoreAPI{}
	command := vfs.PluginCommand{ID: "test.duplicate-command", Label: "One", Run: func(vfs.App) {}}
	registration, err := api.RegisterPluginCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)
	duplicate := command
	duplicate.ID = "TEST.DUPLICATE-COMMAND"
	if _, err := api.RegisterPluginCommand(duplicate); err == nil {
		t.Fatal("case-insensitive duplicate command ID was accepted")
	}
}

func TestPluginCommandExecutionReResolvesAfterUnregister(t *testing.T) {
	api := &CoreAPI{}
	called := 0
	registration, err := api.RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.stale-menu-command",
		Location: vfs.PluginCommandPanel,
		Label:    "Stale menu command",
		Run:      func(vfs.App) { called++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plughost.ExecutePluginCommand(vfs.PluginCommandPanel, "test.stale-menu-command", nil) || called != 1 {
		t.Fatalf("live command execution: called=%d", called)
	}
	registration.Unregister()
	if plughost.ExecutePluginCommand(vfs.PluginCommandPanel, "test.stale-menu-command", nil) || called != 1 {
		t.Fatalf("unregistered command executed: called=%d", called)
	}
}

func TestPluginCommandRegistrationClonesMetadata(t *testing.T) {
	api := &CoreAPI{}
	searchKeys := []string{"Test.PluginCommand.Search"}
	searchTerms := []string{"literal alias"}
	localizedLabels := map[string]string{"ru": "Localized label"}
	localizedDescriptions := map[string]string{"ru": "Localized description"}
	registration, err := api.RegisterPluginCommand(vfs.PluginCommand{
		ID:                    "test.plugin-command-metadata-copy",
		Label:                 "Metadata copy",
		SearchKeys:            searchKeys,
		SearchTerms:           searchTerms,
		LocalizedLabels:       localizedLabels,
		LocalizedDescriptions: localizedDescriptions,
		Run:                   func(vfs.App) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	searchKeys[0] = "mutated by plugin"
	searchTerms[0] = "mutated by plugin"
	localizedLabels["ru"] = "mutated by plugin"
	localizedDescriptions["ru"] = "mutated by plugin"
	commands := plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, nil)
	if len(commands) != 1 ||
		!reflect.DeepEqual(commands[0].SearchKeys, []string{"Test.PluginCommand.Search"}) ||
		!reflect.DeepEqual(commands[0].SearchTerms, []string{"literal alias"}) ||
		!reflect.DeepEqual(commands[0].LocalizedLabels, map[string]string{"ru": "Localized label"}) ||
		!reflect.DeepEqual(commands[0].LocalizedDescriptions, map[string]string{"ru": "Localized description"}) {
		t.Fatalf("registered metadata = %#v", commands)
	}

	commands[0].SearchKeys[0] = "mutated snapshot"
	commands[0].SearchTerms[0] = "mutated snapshot"
	commands[0].LocalizedLabels["ru"] = "mutated snapshot"
	commands[0].LocalizedDescriptions["ru"] = "mutated snapshot"
	commands = plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, nil)
	if len(commands) != 1 ||
		!reflect.DeepEqual(commands[0].SearchKeys, []string{"Test.PluginCommand.Search"}) ||
		!reflect.DeepEqual(commands[0].SearchTerms, []string{"literal alias"}) ||
		!reflect.DeepEqual(commands[0].LocalizedLabels, map[string]string{"ru": "Localized label"}) ||
		!reflect.DeepEqual(commands[0].LocalizedDescriptions, map[string]string{"ru": "Localized description"}) {
		t.Fatalf("snapshot mutation reached registry: %#v", commands)
	}
}

func TestPluginCommandOwnedLocalizationUsesCurrentAndFallbackLanguages(t *testing.T) {
	oldLanguage := config.App.Language
	oldFallbackLanguage := config.App.FallbackLanguage
	t.Cleanup(func() {
		config.App.Language = oldLanguage
		config.App.FallbackLanguage = oldFallbackLanguage
	})

	command := vfs.PluginCommand{
		Label:                 "English fallback label",
		LocalizedLabels:       map[string]string{"fr-CA": "Libelle francais", "de": "Deutsche Beschriftung"},
		Description:           "English fallback description",
		LocalizedDescriptions: map[string]string{"fr-CA": "Description francaise", "de": "Deutsche Beschreibung"},
		SearchTerms:           []string{"literal alias"},
	}

	config.App.Language = "fr_CA"
	config.App.FallbackLanguage = "de"
	if got := plughost.PluginCommandDisplayLabel(command); got != "Libelle francais" {
		t.Fatalf("regional current-language label = %q", got)
	}
	if got := plughost.PluginCommandDisplayDescription(command); got != "Description francaise" {
		t.Fatalf("regional current-language description = %q", got)
	}

	config.App.Language = "it"
	if got := plughost.PluginCommandDisplayLabel(command); got != "Deutsche Beschriftung" {
		t.Fatalf("fallback-language label = %q", got)
	}
	if got := plughost.PluginCommandSearchTerms(command); !reflect.DeepEqual(got, []string{
		"literal alias",
		"Deutsche Beschriftung",
		"Libelle francais",
		"Deutsche Beschreibung",
		"Description francaise",
	}) {
		t.Fatalf("all-language search terms = %#v", got)
	}
}

func TestPluginCommandExecutionRejectsClosedPanelsFrame(t *testing.T) {
	api := &CoreAPI{}
	called := 0
	registration, err := api.RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.closed-panels-frame-command",
		Location: vfs.PluginCommandPanel,
		Label:    "Closed frame command",
		Run:      func(vfs.App) { called++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	panels := &panel.PanelsFrame{Closed: true}
	if plughost.ExecutePluginCommand(vfs.PluginCommandPanel, "TEST.CLOSED-PANELS-FRAME-COMMAND", panels) {
		t.Fatal("command executed for a closed PanelsFrame")
	}
	if called != 0 {
		t.Fatalf("closed-frame handler calls = %d", called)
	}
}

func TestPluginCommandDisplayMetadataTracksActiveLanguage(t *testing.T) {
	oldLanguage := config.App.Language
	oldFallbackLanguage := config.App.FallbackLanguage
	t.Cleanup(func() {
		config.App.Language = oldLanguage
		config.App.FallbackLanguage = oldFallbackLanguage
		initLang()
	})

	command := vfs.PluginCommand{
		Label:          "Extract files",
		LabelKey:       "Archive.Command.Extract",
		Description:    "Extract the selected archive to the passive panel",
		DescriptionKey: "Archive.Command.Extract.Desc",
		SearchKeys:     []string{"Attributes.Archive", "Attributes.Archive", ""},
	}

	config.App.FallbackLanguage = ""
	config.App.Language = "ru"
	initLang()
	if got := plughost.PluginCommandDisplayLabel(command); got != "Извлечь файлы" {
		t.Fatalf("Russian label = %q", got)
	}
	if got := plughost.PluginCommandDisplayDescription(command); got != "Извлечь выбранный архив в пассивную панель" {
		t.Fatalf("Russian description = %q", got)
	}
	if got := plughost.PluginCommandTranslationKeys(command); !reflect.DeepEqual(got, []string{
		"Archive.Command.Extract",
		"Archive.Command.Extract.Desc",
		"Attributes.Archive",
	}) {
		t.Fatalf("translation keys = %#v", got)
	}

	config.App.Language = "en"
	initLang()
	if got := plughost.PluginCommandDisplayLabel(command); got != command.Label {
		t.Fatalf("English label = %q, want %q", got, command.Label)
	}
	missing := vfs.PluginCommand{Label: "Fallback label", LabelKey: "Test.Missing.PluginCommand.Label"}
	if got := plughost.PluginCommandDisplayLabel(missing); got != missing.Label {
		t.Fatalf("missing localization returned %q, want fallback %q", got, missing.Label)
	}
}

func TestPluginConfigurationActionIsRegistered(t *testing.T) {
	action, ok := GetAction("Settings.PluginConfiguration")
	if !ok || action.Handler == nil || action.MenuPath != "Options" {
		t.Fatalf("plugin configuration action = %#v, registered=%t", action, ok)
	}
}
