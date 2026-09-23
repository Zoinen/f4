package netfox

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
)

type recordDialogApp struct {
	netFoxPluginTestApp
	calls              int
	collection, record string
	create             bool
}

func (a *recordDialogApp) OpenSettingsRecord(collection, record string, create bool) bool {
	a.calls++
	a.collection, a.record, a.create = collection, record, create
	return true
}

func TestAddConnectionSemanticActivation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	w := &netFoxVFSWrapper{NewNetFoxVFS(t.TempDir() + "/connections.json")}
	a := &recordDialogApp{netFoxPluginTestApp: netFoxPluginTestApp{active: w, selected: "<Add connection>"}}
	for _, action := range []vfs.PanelAction{vfs.PanelActionActivate, vfs.PanelActionCreate} {
		if !w.HandlePanelAction(a, action, []string{w.Join(w.GetPath(), "<Add connection>")}) {
			t.Fatal("Add connection was not handled")
		}
		if a.collection != "netfox.connections" || a.record != "" || !a.create {
			t.Fatalf("incorrect record route: %+v", a)
		}
	}
	if w.HandlePanelAction(a, vfs.PanelActionActivate, []string{"server"}) {
		t.Fatal("ordinary connection activation was swallowed")
	}
	var items []vfs.VFSItem
	err := w.ReadDir(context.Background(), w.GetPath(), func(batch []vfs.VFSItem) { items = append(items, batch...) })
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].IconKey != "plus" || !items[0].NoExtension {
		t.Fatalf("Add item metadata: %+v", items)
	}
}
