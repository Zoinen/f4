package filegroups

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestV2FactoryScopeMigrationPreservesUniversalFeatures(t *testing.T) {
	factory := []Group{}
	for _, scope := range []string{"lucide", "zoin", "system"} {
		factory = append(factory, Group{ID: "factory." + scope, Fields: map[string]string{
			"LegacyID": "old-factory", "IconTheme": scope, "Factory": "true", "Mask": "*.txt", "Icons.Enabled": "true", "Icon.*": "asset:file",
		}})
	}
	old := Group{ID: "old-factory", Fields: map[string]string{
		"Name": "Customized", "Mask": "*.go", "Highlight.Enabled": "true", "NormalColor": "foreground:#123456",
		"UseDefaults": "true", "ContinueProcessing": "true", "Sort.Enabled": "true", "Sort.Number": "3",
		"Associations.Enabled": "true", "Vendor.Keep": "unknown", "Icon.future": "asset:future",
	}, Commands: []CommandSet{{ID: "app", Name: "App", Commands: [6]string{`C:\Tools\open.exe`, "two\nlines", "v", "av", "e", "ae"}, Enabled: [6]bool{true, true, true, true, true, true}}}}
	doc, err := Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := doc.Encode([]Group{old, {ID: "qt.zoin.user", Fields: map[string]string{"Mask": "*.go", "Highlight.Enabled": "true"}}})
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "Version = 3", "Version = 2", 1))
	path := filepath.Join(t.TempDir(), "highlight.ini")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path, Defaults: factory}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if !tx.Migrated() {
		t.Fatal("v2 migration not reported")
	}
	if unchanged, _ := os.ReadFile(path); !reflect.DeepEqual(unchanged, data) {
		t.Fatal("Begin wrote migration")
	}
	groups := tx.Groups()
	if len(groups) != 5 {
		t.Fatalf("groups=%+v", groups)
	}
	snapshot, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"lucide", "zoin", "system", "future"} {
		view := snapshot.ForIconTheme(scope)
		associations := view.Matching(&vfs.VFSItem{Name: "x.go"}, true, "Associations", time.Now())
		if len(associations) != 1 || associations[0].ID != "old-factory.universal" || len(associations[0].Commands) != 1 {
			t.Fatalf("%s lost/duplicated commands: %+v", scope, associations)
		}
		command := associations[0].Commands[0]
		if command.Name != old.Commands[0].Name || command.Commands != old.Commands[0].Commands || command.Enabled != old.Commands[0].Enabled {
			t.Fatalf("%s changed command slots: %+v", scope, command)
		}
		if snapshot.Predicate("qt.zoin.user") == nil {
			t.Fatal("user prefix inferred as factory")
		}
		for _, group := range view.Groups() {
			if group.IconTheme() != "" && (group.Enabled("Highlight") || group.Enabled("Sort") || group.Enabled("Associations") || len(group.Commands) > 0) {
				t.Fatal("scoped group retained universal features")
			}
		}
	}
	if err := tx.Commit(groups); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".pre-filegroups-v3")
	if err != nil || !reflect.DeepEqual(data, backup) {
		t.Fatal("missing exact v2 backup", err)
	}
	saved, _ := os.ReadFile(path)
	if !strings.Contains(string(saved), "Version = 3") {
		t.Fatal("unsafe output version")
	}
	reloaded, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Migrated() || !reflect.DeepEqual(reloaded.Groups(), groups) {
		t.Fatal("v3 restart changed groups or repeated migration")
	}
	if err := reloaded.Commit(reloaded.Groups()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(saved, []byte("\n# external edit\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reloaded.Commit(groups); !errors.Is(err, ErrConflict) {
		t.Fatalf("external change overwritten: %v", err)
	}
}

func TestScopeMigrationIdentityCollisionIsDiagnostic(t *testing.T) {
	_, err := migrateThemeScopes([]Group{{ID: "builtin", Fields: map[string]string{"LegacyID": "builtin", "IconTheme": "zoin"}}},
		[]Group{{ID: "builtin", Fields: map[string]string{"Highlight.Enabled": "true"}}, {ID: "builtin.universal", Fields: map[string]string{}}})
	if err == nil {
		t.Fatal("existing user identity overwritten")
	}
}
