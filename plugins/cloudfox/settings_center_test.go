package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/unxed/f4/sdk/f4settings"
	"testing"
)

func TestSettingsMetadataDraftPartialSaveAndUnknownFields(t *testing.T) {
	ctx := context.Background()
	p := NewPlugin(Options{ConfigDir: t.TempDir(), Keyring: failingSecretStore{errors.New("must not unlock")}, Vault: failingSecretStore{errors.New("must not unlock")}})
	create := func(name string) Connection {
		c, err := p.repo.Connections.Create(ctx, Connection{Name: name, Provider: ProviderWebDAV, Settings: json.RawMessage(`{"base_url":"https://example.org","root":"/","auth":"anonymous","future":"keep"}`)})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	a, b := create("Profile A"), create("Profile B")
	provider := &centerSettingsProvider{plugin: p}
	if err := f4settings.ValidateCatalog(provider.Catalog()); err != nil {
		t.Fatal(err)
	}
	d, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for i := range d.Records["cloudfox.webdav"] {
		r := &d.Records["cloudfox.webdav"][i]
		r.Values["cloudfox.webdav.name"] += " edited"
	}
	b.Name = "Concurrent B"
	if _, err := p.repo.Connections.Update(ctx, b); err != nil {
		t.Fatal(err)
	}
	result := d.Commit(ctx)
	if len(result.Errors) != 1 || result.Errors[b.ID] == nil {
		t.Fatalf("partial result: %#v", result)
	}
	saved, err := p.repo.Get(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Profile A edited" {
		t.Fatal(saved.Name)
	}
	var raw map[string]any
	_ = json.Unmarshal(saved.Settings, &raw)
	if raw["future"] != "keep" {
		t.Fatal("unknown provider field lost")
	}
	if !d.Dirty("cloudfox.webdav") {
		t.Fatal("failed record draft was discarded")
	}
	for _, r := range d.BaselineRecords["cloudfox.webdav"] {
		if r.ID == a.ID && r.Values["cloudfox.webdav.name"] != "Profile A edited" {
			t.Fatal("successful record not rebased")
		}
	}
	if _, err := p.repo.Connections.DeleteIfCurrent(ctx, b); !errors.Is(err, ErrConnectionChanged) {
		t.Fatalf("stale delete: %v", err)
	}
}
