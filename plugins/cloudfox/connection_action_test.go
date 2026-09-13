package cloudfox

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
)

type connectionDialogApp struct {
	vfs.App
	collection, record string
	create             bool
	choices            []string
	choose             func(int)
}

func (a *connectionDialogApp) OpenSettingsRecord(collection, record string, create bool) bool {
	a.collection, a.record, a.create = collection, record, create
	return true
}
func (a *connectionDialogApp) Menu(_ string, choices []string, choose func(int)) {
	a.choices, a.choose = choices, choose
}

func TestConnectionDialogSelectsOnlyRequestedProvider(t *testing.T) {
	p := NewPlugin(Options{ConfigDir: t.TempDir(), Portable: true, Factories: []BackendFactory{&GoogleDriveFactory{}}})
	defer p.Close()
	m := p.manager()
	a := &connectionDialogApp{}
	e := &simpleProfileEditor{plugin: p}
	e.EditProfile(a, m, nil)
	if len(a.choices) != 1 || a.choose == nil || a.collection != "" {
		t.Fatal("provider choice missing")
	}
	a.choose(0)
	if a.collection != "cloudfox.gdrive" || a.record != "" || !a.create {
		t.Fatalf("wrong new record route: %+v", a)
	}
	e.EditProfile(a, m, &Connection{ID: "existing", Provider: ProviderGoogleDrive})
	if a.collection != "cloudfox.gdrive" || a.record != "existing" || a.create {
		t.Fatalf("wrong edit route: %+v", a)
	}
	var items []vfs.VFSItem
	if err := m.ReadDir(context.Background(), m.GetPath(), func(batch []vfs.VFSItem) { items = append(items, batch...) }); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].IconKey != "plus" {
		t.Fatalf("Add icon missing: %+v", items)
	}
}
