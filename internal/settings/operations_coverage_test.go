package settings

import (
	"context"
	"strings"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
)

func operationsCatalogForTest(t *testing.T) f4settings.Catalog {
	t.Helper()
	catalog := (settingsOperationsProvider{}).Catalog()
	if err := f4settings.ValidateCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestSettingsOperationsCatalogIsValid(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	if catalog.ID != "operations" || len(catalog.Commands) < 10 || len(catalog.Fields) != 6 {
		t.Fatalf("operations catalog shape = commands %d fields %d id %q", len(catalog.Commands), len(catalog.Fields), catalog.ID)
	}
}

func TestSettingsOperationsCatalogHasManualSaveCommands(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	for _, id := range []string{"save.preferences", "save.session", "save.geometry"} {
		found := false
		for _, command := range catalog.Commands {
			if command.ID == id {
				found = true
				if command.Background {
					t.Fatalf("manual save command %q is background", id)
				}
			}
		}
		if !found {
			t.Fatalf("missing manual save command %q", id)
		}
	}
}

func TestSettingsOperationsCatalogHasSyntaxCommands(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	for _, id := range []string{"syntax.reload", "syntax.check", "syntax.download"} {
		for _, command := range catalog.Commands {
			if command.ID == id {
				if !command.Background || len(command.Requires) == 0 {
					t.Fatalf("syntax command %q has no background/requirements metadata", id)
				}
				goto found
			}
		}
		t.Fatalf("missing syntax command %q", id)
	found:
	}
}

func TestSettingsOperationsCatalogHasProfileVariants(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	seen := map[string]bool{}
	for _, command := range catalog.Commands {
		if strings.HasPrefix(command.ID, "profile.") {
			seen[command.ID] = true
			if len(command.Requires) != 1 || command.Requires[0] != "*" {
				t.Fatalf("profile command %q requirements = %#v", command.ID, command.Requires)
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("profile command variants = %#v, want four", seen)
	}
}

func TestSettingsOperationsCatalogFieldsHaveDefaults(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	for _, field := range catalog.Fields {
		if field.ID == "" || field.Default == "" || field.Unavailable == "" {
			t.Fatalf("incomplete operations field = %#v", field)
		}
	}
}

func TestSettingsOperationsCatalogUpdateCommandRequirements(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	for _, command := range catalog.Commands {
		if command.ID == "updates.check" {
			if !command.Background || len(command.Requires) != 6 {
				t.Fatalf("update command metadata = %#v", command)
			}
			return
		}
	}
	t.Fatal("missing updates.check command")
}

func TestSettingsOperationsCatalogColorExportRequirements(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	for _, command := range catalog.Commands {
		if command.ID == "colors.export" {
			if command.Background || len(command.Requires) != 2 {
				t.Fatalf("color export metadata = %#v", command)
			}
			return
		}
	}
	t.Fatal("missing colors.export command")
}

func TestSettingsOperationsArchiveClearCommandRuns(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	for _, command := range catalog.Commands {
		if command.ID == "archives.clearTarIndexes" {
			if err := command.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing archives.clearTarIndexes command")
}

func TestSettingsOperationsBeginCreatesDraft(t *testing.T) {
	draft, err := (settingsOperationsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	if draft.Values["updates.status"] != "Never" {
		t.Fatalf("updates.status default = %q", draft.Values["updates.status"])
	}
	if len(draft.Values) != 6 {
		t.Fatalf("draft values = %d, want six", len(draft.Values))
	}
}

func TestSettingsOperationsCatalogCategoriesAndGroups(t *testing.T) {
	catalog := operationsCatalogForTest(t)
	for _, command := range catalog.Commands {
		if command.Category == "" || command.Group == "" || command.Label.English == "" || command.Description.English == "" {
			t.Fatalf("incomplete operation command = %#v", command)
		}
	}
}
