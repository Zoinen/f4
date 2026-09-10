package plughost

import (
	"testing"
)

func TestCurrentAppWithoutApplication(t *testing.T) {
	old := App
	App = nil
	t.Cleanup(func() { App = old })
	if currentApp() != nil {
		t.Fatal("currentApp returned an app without a host application")
	}
}

func TestRPCPluginConstructorsAndIdentity(t *testing.T) {
	plugin := NewRPCPlugin("/opt/f4/notes")
	if plugin.path != "/opt/f4/notes" || plugin.dir != "" || plugin.GetName() != "/opt/f4/notes (RPC)" {
		t.Fatalf("NewRPCPlugin produced %+v", plugin)
	}
	if got := plugin.permissionIdentity(); got.Key != "/opt/f4/notes" || got.Title != "notes" {
		t.Fatalf("path identity = %+v", got)
	}

	ringPlugin := NewRPCPlugRing("/opt/f4", "notes")
	if ringPlugin.path != "notes" || ringPlugin.dir != "/opt/f4" {
		t.Fatalf("NewRPCPlugRing produced %+v", ringPlugin)
	}

	want := PluginIdentity{Key: "catalog-notes", Title: "Notes"}
	plugin.SetPermissionIdentity(want)
	if got := plugin.permissionIdentity(); got.Key != want.Key || got.Title != want.Title {
		t.Fatalf("explicit identity = %+v, want %+v", got, want)
	}
	if err := plugin.Close(); err != nil {
		t.Fatalf("closing an unstarted RPC plugin: %v", err)
	}
}
