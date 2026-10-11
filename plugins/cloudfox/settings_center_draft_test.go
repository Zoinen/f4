package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
)

// newDraftTestPlugin builds a Plugin whose credential storage is entirely
// in-memory: the OS keyring must never be touched (it fails loudly if it
// is), and the vault is a deterministic map so tests can assert on the
// exact secret reference that gets created and removed.
func newDraftTestPlugin(t *testing.T) *Plugin {
	t.Helper()
	return NewPlugin(Options{
		ConfigDir: t.TempDir(),
		Portable:  true,
		Keyring:   failingSecretStore{errors.New("must not unlock")},
		Vault:     &memorySecretStore{},
	})
}

func webdavRecordValues(name, baseURL, auth string) map[string]string {
	const prefix = "cloudfox.webdav."
	return map[string]string{
		prefix + "name":        name,
		prefix + "credentials": "keep",
		prefix + "base_url":    baseURL,
		prefix + "root":        "/",
		prefix + "auth":        auth,
	}
}

// TestCenterSettingsValidateFunc exercises the duplicate-name and
// invalid-connection error paths of the settings-center Draft's
// ValidateFunc, which previously had no direct coverage at all.
func TestCenterSettingsValidateFunc(t *testing.T) {
	ctx := context.Background()
	// "edited" always keys the wanted error to a.ID; "either" means exactly
	// one of the two records must fail (ValidateFunc walks d.Records[col.ID]
	// in whatever order the collection currently holds them, so a duplicate
	// pair can be flagged on either ID depending on that order).
	tests := []struct {
		name      string
		mutate    func(r *f4settings.Record, other *f4settings.Record)
		wantErrOn string // "edited", "either", or "" for none
		wantDup   bool
	}{
		{
			name: "case-insensitive duplicate name is rejected",
			mutate: func(r, other *f4settings.Record) {
				r.Values["cloudfox.webdav.name"] = strings.ToUpper(other.Values["cloudfox.webdav.name"])
			},
			wantErrOn: "either",
			wantDup:   true,
		},
		{
			name: "blank name surfaces the underlying validation error",
			mutate: func(r, other *f4settings.Record) {
				r.Values["cloudfox.webdav.name"] = "   "
			},
			wantErrOn: "edited",
			wantDup:   false,
		},
		{
			name: "distinct, valid names pass",
			mutate: func(r, other *f4settings.Record) {
				r.Values["cloudfox.webdav.name"] = "Renamed Drop"
			},
			wantErrOn: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newDraftTestPlugin(t)
			settingsJSON := json.RawMessage(`{"base_url":"https://example.org/dav","root":"/","auth":"anonymous"}`)
			a, err := p.repo.Connections.Create(ctx, Connection{Name: "Archive Drop", Provider: ProviderWebDAV, Settings: settingsJSON})
			if err != nil {
				t.Fatal(err)
			}
			b, err := p.repo.Connections.Create(ctx, Connection{Name: "Backup Drop", Provider: ProviderWebDAV, Settings: settingsJSON})
			if err != nil {
				t.Fatal(err)
			}

			provider := &centerSettingsProvider{plugin: p}
			d, err := provider.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()

			rows := d.Records["cloudfox.webdav"]
			var edited, other *f4settings.Record
			for i := range rows {
				switch rows[i].ID {
				case a.ID:
					edited = &rows[i]
				case b.ID:
					other = &rows[i]
				}
			}
			if edited == nil || other == nil {
				t.Fatalf("expected both seeded connections in the draft, got %#v", rows)
			}
			tt.mutate(edited, other)

			failures := d.Validate()

			if tt.wantErrOn == "" {
				if len(failures) != 0 {
					t.Fatalf("expected no validation failures, got %#v", failures)
				}
				return
			}
			if len(failures) != 1 {
				t.Fatalf("expected exactly one validation failure, got %#v", failures)
			}
			var got error
			switch tt.wantErrOn {
			case "edited":
				got = failures[a.ID]
				if got == nil {
					t.Fatalf("expected the failure to be keyed on the edited record %s, got %#v", a.ID, failures)
				}
			case "either":
				if failures[a.ID] != nil {
					got = failures[a.ID]
				} else {
					got = failures[b.ID]
				}
				if got == nil {
					t.Fatalf("expected the failure to be keyed on either record, got %#v", failures)
				}
			}
			if tt.wantDup && !errors.Is(got, ErrDuplicateName) {
				t.Fatalf("expected ErrDuplicateName, got %v", got)
			}
			if !tt.wantDup && errors.Is(got, ErrDuplicateName) {
				t.Fatalf("did not expect ErrDuplicateName, got %v", got)
			}
		})
	}
}

// TestCenterSettingsCommitCreatesAndDeletesRecord drives the CommitFunc
// closure through both branches that previously had no coverage: creating
// a brand-new connection (assigning it a real ID and a vault-backed
// secret reference, even though anonymous WebDAV stores no credentials),
// and then deleting it once the record disappears from the draft. This
// also confirms the secret bundle is actually removed from the vault, not
// just the connection metadata.
func TestCenterSettingsCommitCreatesAndDeletesRecord(t *testing.T) {
	ctx := context.Background()
	p := newDraftTestPlugin(t)
	provider := &centerSettingsProvider{plugin: p}

	d, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	const draftID = "new-1"
	d.Records["cloudfox.webdav"] = append(d.Records["cloudfox.webdav"], f4settings.Record{
		ID:     draftID,
		Values: webdavRecordValues("Fresh Drop", "https://example.org/dav", "anonymous"),
	})

	result := d.Commit(ctx)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors creating the record: %#v", result.Errors)
	}
	foundApplied := false
	for _, id := range result.Applied {
		if id == draftID {
			foundApplied = true
		}
	}
	if !foundApplied {
		t.Fatalf("expected %q in Applied, got %v", draftID, result.Applied)
	}

	items, err := p.repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly one stored connection, got %d", len(items))
	}
	created := items[0]
	if created.Name != "Fresh Drop" {
		t.Fatalf("unexpected connection name %q", created.Name)
	}
	if created.SecretRef == "" || !strings.HasPrefix(created.SecretRef, "vault:") {
		t.Fatalf("expected a vault-backed secret reference, got %q", created.SecretRef)
	}
	if _, err := p.repo.Secrets.Get(ctx, created.SecretRef); err != nil {
		t.Fatalf("secret bundle should exist right after creation: %v", err)
	}

	// Simulate the UI removing the row: the draft-local record disappears
	// from Records while the baseline (rebased by Accept above) still has
	// it, which is exactly what the "present[base.ID]" bookkeeping in
	// CommitFunc is meant to detect.
	d.Records["cloudfox.webdav"] = nil

	result = d.Commit(ctx)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors deleting the record: %#v", result.Errors)
	}
	deleted := false
	for _, id := range result.Deleted {
		if id == draftID {
			deleted = true
		}
	}
	if !deleted {
		t.Fatalf("expected %q in Deleted, got %v", draftID, result.Deleted)
	}

	items, err = p.repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("expected the connection to be gone, still have %#v", items)
	}
	if _, err := p.repo.Secrets.Get(ctx, created.SecretRef); err == nil {
		t.Fatal("expected the vault-backed secret to be removed along with the connection")
	}
}

// TestCenterSettingsCommitSkipsUnchangedRecord confirms the
// reflect.DeepEqual "unchanged" fast path in CommitFunc actually skips
// re-saving a sibling record that was not touched, rather than rewriting
// every dirty collection's rows unconditionally: an untouched record must
// keep its original UpdatedAt and must not be reported as individually
// Applied.
func TestCenterSettingsCommitSkipsUnchangedRecord(t *testing.T) {
	ctx := context.Background()
	p := newDraftTestPlugin(t)
	settingsJSON := json.RawMessage(`{"base_url":"https://example.org/dav","root":"/","auth":"anonymous"}`)
	a, err := p.repo.Connections.Create(ctx, Connection{Name: "Archive Drop", Provider: ProviderWebDAV, Settings: settingsJSON})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.repo.Connections.Create(ctx, Connection{Name: "Backup Drop", Provider: ProviderWebDAV, Settings: settingsJSON})
	if err != nil {
		t.Fatal(err)
	}

	provider := &centerSettingsProvider{plugin: p}
	d, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	for i := range d.Records["cloudfox.webdav"] {
		if d.Records["cloudfox.webdav"][i].ID == a.ID {
			d.Records["cloudfox.webdav"][i].Values["cloudfox.webdav.name"] = "Archive Drop Renamed"
		}
	}

	result := d.Commit(ctx)
	if len(result.Errors) != 0 {
		t.Fatalf("unexpected errors: %#v", result.Errors)
	}
	for _, id := range result.Applied {
		if id == b.ID {
			t.Fatalf("untouched record %s must not be individually Applied, got %v", b.ID, result.Applied)
		}
	}

	afterA, err := p.repo.Get(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterA.Name != "Archive Drop Renamed" {
		t.Fatalf("edited record was not saved: %q", afterA.Name)
	}
	afterB, err := p.repo.Get(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !afterB.UpdatedAt.Equal(b.UpdatedAt) {
		t.Fatalf("untouched record must not be rewritten: before %v, after %v", b.UpdatedAt, afterB.UpdatedAt)
	}
}
