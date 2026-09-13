package settings

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

type recordDialogProvider struct {
	catalog                   f4settings.Catalog
	opened, closed, committed int
}

func (p *recordDialogProvider) Catalog() f4settings.Catalog { return p.catalog }
func (p *recordDialogProvider) Begin(context.Context) (*f4settings.Draft, error) {
	p.opened++
	d := f4settings.NewDraft(nil, map[string][]f4settings.Record{
		"test.connections": {{ID: "existing", Values: map[string]string{"name": "Existing"}}},
		"test.other":       {{ID: "sibling", Values: map[string]string{"name": "Untouched"}}},
	})
	d.CommitFunc = func(context.Context, *f4settings.Draft) f4settings.Result {
		p.committed++
		return f4settings.Result{Applied: []string{"test.connections"}}
	}
	d.CloseFunc = func() { p.closed++ }
	return d, nil
}

func TestRecordDialogIsolatedEditor(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fields := []f4settings.Field{recordField("name", "Name", "Connection name.", f4settings.String)}
	p := &recordDialogProvider{catalog: f4settings.Catalog{ID: "record-test", Categories: Categories,
		Collections: []f4settings.Collection{
			recordCollection("test.connections", "network", "Connections", "Test connections.", "name", fields),
			recordCollection("test.other", "network", "Others", "Other connections.", "other-name", []f4settings.Field{recordField("other-name", "Name", "Other name.", f4settings.String)}),
		}}}
	p.catalog.Collections[0].Actions = []f4settings.RecordCommand{{ID: "test.authorize", Label: f4settings.Text{English: "Authorize"}, Description: f4settings.Text{English: "Test authorization action."}, Run: func(context.Context, f4settings.Record) (map[string]string, error) { return nil, nil }}}
	reg, err := RegisterProvider(p)
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Unregister()
	for _, create := range []bool{true, false} {
		if !OpenRecord("test.connections", "existing", create, nil) {
			t.Fatal("not opened")
		}
		c := vtui.FrameManager.GetTopFrame().(*settingsCenter)
		c.SetPosition(0, 0, 99, 39)
		if c.sidebar.IsVisible() || c.search.IsVisible() {
			t.Fatal("global settings chrome visible")
		}
		found := false
		for _, row := range c.page.rows {
			if row.control == nil {
				continue
			}
			if row.control.GetId() == "collection:test.connections" {
				t.Fatal("record list visible")
			}
			if row.field.ID == "name" {
				found = true
			}
		}
		if !found {
			t.Fatal("connection field missing")
		}
		ids := map[any]bool{}
		for _, node := range settingsSemanticNodes(c.SemanticNode(&vtui.SemanticContext{Width: 100, Height: 40})) {
			if node["kind"] == "button" {
				if ids[node["id"]] {
					t.Fatalf("duplicate button target %v", node["id"])
				}
				ids[node["id"]] = true
			}
			if node["layoutRole"] == "navigation" || node["layoutRole"] == "search" {
				t.Fatal("global semantic chrome present")
			}
		}
		d := c.sessions[0].draft
		want := 1
		if create {
			want = 2
		}
		if len(d.Records["test.connections"]) != want || len(d.Records["test.other"]) != 1 {
			t.Fatal("draft lost sibling records or did not add record")
		}
		c.Close()
	}
	if p.closed != 2 || p.committed != 0 {
		t.Fatalf("cancel lifecycle: %+v", p)
	}
	applied := 0
	if !OpenRecord("test.connections", "existing", false, func() { applied++ }) {
		t.Fatal("edit not opened")
	}
	c := vtui.FrameManager.GetTopFrame().(*settingsCenter)
	c.sessions[0].draft.Records["test.connections"][0].Values["name"] = "Changed"
	c.commit(true)
	if p.committed != 1 || p.closed != 3 || applied != 1 {
		t.Fatalf("save lifecycle: %+v applied=%d", p, applied)
	}
	if OpenRecord("test.connections", "missing", false, nil) {
		t.Fatal("missing record opened another connection")
	}
}
