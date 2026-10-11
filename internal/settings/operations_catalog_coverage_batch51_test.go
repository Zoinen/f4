package settings

import (
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
)

func operationsCatalogCoverageBatch51(t *testing.T) f4settings.Catalog {
	t.Helper()
	return (settingsOperationsProvider{}).Catalog()
}

func findOperationsCommandCoverageBatch51(t *testing.T, cat f4settings.Catalog, id string) f4settings.Command {
	t.Helper()
	for _, command := range cat.Commands {
		if command.ID == id {
			return command
		}
	}
	t.Fatalf("settings command %q was not registered", id)
	return f4settings.Command{}
}

func hasRequiredFieldCoverageBatch51(requires []string, want string) bool {
	for _, got := range requires {
		if got == want {
			return true
		}
	}
	return false
}

func TestOperationsCatalogRegistersSavePreferencesCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "save.preferences")
	if command.Background {
		t.Fatal("save.preferences must be a foreground command")
	}
}

func TestOperationsCatalogRegistersSaveSessionCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "save.session")
	if command.Category != "workspaces" {
		t.Fatalf("save.session category = %q, want workspaces", command.Category)
	}
}

func TestOperationsCatalogRegistersSaveGeometryCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "save.geometry")
	if command.Category != "workspaces" {
		t.Fatalf("save.geometry category = %q, want workspaces", command.Category)
	}
}

func TestOperationsCatalogRegistersColorsExportRequirementsCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "colors.export")
	if !hasRequiredFieldCoverageBatch51(command.Requires, "ColorStyle") || !hasRequiredFieldCoverageBatch51(command.Requires, "EnforceColorCorrection") {
		t.Fatalf("colors.export requires = %v", command.Requires)
	}
}

func TestOperationsCatalogRegistersSyntaxReloadAsBackgroundCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "syntax.reload")
	if !command.Background || !hasRequiredFieldCoverageBatch51(command.Requires, "EditorColorerScheme") {
		t.Fatalf("syntax.reload = background %v, requires %v", command.Background, command.Requires)
	}
}

func TestOperationsCatalogRegistersSyntaxCheckAsBackgroundCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "syntax.check")
	if !command.Background {
		t.Fatal("syntax.check must be a background command")
	}
}

func TestOperationsCatalogRegistersSyntaxDownloadAsBackgroundCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "syntax.download")
	if !command.Background || !hasRequiredFieldCoverageBatch51(command.Requires, "ProxyMode") {
		t.Fatalf("syntax.download = background %v, requires %v", command.Background, command.Requires)
	}
}

func TestOperationsCatalogRegistersUpdateCheckRequirementsCoverageBatch51(t *testing.T) {
	command := findOperationsCommandCoverageBatch51(t, operationsCatalogCoverageBatch51(t), "updates.check")
	if !command.Background || !hasRequiredFieldCoverageBatch51(command.Requires, "UpdateChannel") {
		t.Fatalf("updates.check = background %v, requires %v", command.Background, command.Requires)
	}
}

func TestOperationsCatalogRegistersProfileLocationFieldsCoverageBatch51(t *testing.T) {
	cat := operationsCatalogCoverageBatch51(t)
	want := map[string]bool{"profile.path": false, "profile.ini": false, "profile.session": false, "profile.portable": false, "profile.system": false}
	for _, field := range cat.Fields {
		if _, ok := want[field.ID]; ok {
			want[field.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("catalog is missing field %q", id)
		}
	}
}

func TestOperationsCatalogMarksProfileCommandsAsRequiringAllSettingsCoverageBatch51(t *testing.T) {
	cat := operationsCatalogCoverageBatch51(t)
	for _, id := range []string{"profile.true.false", "profile.true.true", "profile.false.false", "profile.false.true"} {
		command := findOperationsCommandCoverageBatch51(t, cat, id)
		if len(command.Requires) != 1 || command.Requires[0] != "*" {
			t.Fatalf("%s requires = %v, want [*]", id, command.Requires)
		}
	}
}
