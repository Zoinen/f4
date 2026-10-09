package envman

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
)

func newSettingsProviderTestPlugin(t *testing.T, config Config) (*Plugin, *Store) {
	t.Helper()
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(config); err != nil {
		t.Fatal(err)
	}
	return &Plugin{store: store, options: EngineOptions{}}, store
}

func TestSettingsProviderCatalogIsComplete(t *testing.T) {
	catalog := (&settingsProvider{plugin: &Plugin{}}).Catalog()
	if err := f4settings.ValidateCatalog(catalog); err != nil {
		t.Fatalf("catalog validation: %v", err)
	}
	if catalog.ID != "envman" || len(catalog.Categories) != 1 || len(catalog.Fields) != 2 || len(catalog.Collections) != 1 || len(catalog.Commands) != 1 {
		t.Fatalf("catalog shape = %#v", catalog)
	}
	if catalog.Collections[0].ID != "envman.profiles" || !catalog.Collections[0].Ordered {
		t.Fatalf("profile collection = %#v", catalog.Collections[0])
	}
	if catalog.Commands[0].ID != "envman.reconcile" {
		t.Fatalf("command = %#v", catalog.Commands[0])
	}
}

func TestSettingsProviderBeginUsesDefaultConfig(t *testing.T) {
	draft, err := (&settingsProvider{plugin: &Plugin{}}).Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if draft.Values["envman.IgnoredVariables"] != "" || draft.Values["envman.AlwaysUseEditor"] != "false" {
		t.Fatalf("default values = %#v", draft.Values)
	}
	if len(draft.Records["envman.profiles"]) != 0 {
		t.Fatalf("default records = %#v", draft.Records)
	}
}

func TestSettingsProviderBeginMapsStoredProfiles(t *testing.T) {
	config := Config{Version: CurrentConfigVersion, IgnoredVariables: []string{"KEEP", "PATH"}, AlwaysUseEditor: true, Entries: []Entry{
		{Kind: KindSeparator},
		{Kind: KindProfile, Name: "work", Enabled: true, Variables: []string{"A=one", "B=two"}},
	}}
	plugin, _ := newSettingsProviderTestPlugin(t, config)
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if draft.Values["envman.IgnoredVariables"] != "KEEP, PATH" || draft.Values["envman.AlwaysUseEditor"] != "true" {
		t.Fatalf("stored values = %#v", draft.Values)
	}
	rows := draft.Records["envman.profiles"]
	if len(rows) != 2 || rows[0].ID != "envman:0" || rows[0].Values["envman.Kind"] != string(KindSeparator) {
		t.Fatalf("stored records = %#v", rows)
	}
	if rows[1].Values["envman.Variables"] != "A=one\nB=two" {
		t.Fatalf("profile variables = %#v", rows[1].Values)
	}
}

func TestSettingsProviderValidatesDraftChanges(t *testing.T) {
	config := testConfig("initial", "A=one")
	plugin, _ := newSettingsProviderTestPlugin(t, config)
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Values["envman.IgnoredVariables"] = "FOO; BAR baz"
	draft.Values["envman.AlwaysUseEditor"] = "true"
	draft.Records["envman.profiles"][0].Values["envman.Variables"] = "A=two\nB=three"
	if validation := draft.Validate(); validation != nil {
		t.Fatalf("valid draft rejected: %#v", validation)
	}
}

func TestSettingsProviderRejectsInvalidDraft(t *testing.T) {
	plugin, _ := newSettingsProviderTestPlugin(t, DefaultConfig())
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Values["envman.IgnoredVariables"] = "not-a-variable"
	validation := draft.Validate()
	if validation["envman"] == nil {
		t.Fatalf("invalid draft validation = %#v", validation)
	}
}

func TestSettingsProviderRejectsExternalIgnoredVariableChange(t *testing.T) {
	config := DefaultConfig()
	config.IgnoredVariables = []string{"KEEP"}
	plugin, store := newSettingsProviderTestPlugin(t, config)
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Values["envman.IgnoredVariables"] = "NEW"
	config.IgnoredVariables = []string{"OUTSIDE"}
	if err := store.Save(config); err != nil {
		t.Fatal(err)
	}
	if validation := draft.Validate(); validation["envman"] == nil {
		t.Fatal("external ignored-variable change was accepted")
	}
}

func TestSettingsProviderRejectsExternalEditorPreferenceChange(t *testing.T) {
	plugin, store := newSettingsProviderTestPlugin(t, DefaultConfig())
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Values["envman.AlwaysUseEditor"] = "true"
	changed := DefaultConfig()
	changed.AlwaysUseEditor = true
	if err := store.Save(changed); err != nil {
		t.Fatal(err)
	}
	if validation := draft.Validate(); validation["envman"] == nil {
		t.Fatal("external editor-preference change was accepted")
	}
}

func TestSettingsProviderRejectsExternalProfileChange(t *testing.T) {
	plugin, store := newSettingsProviderTestPlugin(t, testConfig("initial", "A=one"))
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Records["envman.profiles"][0].Values["envman.Name"] = "draft"
	if err := store.Save(testConfig("outside", "A=outside")); err != nil {
		t.Fatal(err)
	}
	if validation := draft.Validate(); validation["envman"] == nil {
		t.Fatal("external profile change was accepted")
	}
}

func TestSettingsProviderCommitHonorsCanceledContext(t *testing.T) {
	plugin, _ := newSettingsProviderTestPlugin(t, DefaultConfig())
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Values["envman.AlwaysUseEditor"] = "true"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := draft.Commit(ctx)
	if !errors.Is(result.Errors["envman"], context.Canceled) {
		t.Fatalf("canceled commit errors = %#v", result.Errors)
	}
}

func TestSettingsProviderCommitSavesAndApplies(t *testing.T) {
	plugin, store := newSettingsProviderTestPlugin(t, DefaultConfig())
	host := newEnvManTestHost("A=one")
	engine, err := NewEngine([]string{"A=one"}, plugin.options)
	if err != nil {
		t.Fatal(err)
	}
	plugin.engine = engine
	plugin.envHost = host
	draft, err := (&settingsProvider{plugin: plugin}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	draft.Values["envman.AlwaysUseEditor"] = "true"
	result := draft.Commit(context.Background())
	if len(result.Errors) != 0 || len(result.Applied) != 1 {
		t.Fatalf("commit result = %#v", result)
	}
	if !store.Snapshot().AlwaysUseEditor {
		t.Fatal("commit did not persist editor preference")
	}
}
