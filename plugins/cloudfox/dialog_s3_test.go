package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/unxed/vtui"
)

type dialogConnectionProbeBackend struct {
	Backend
	probeCalled bool
	statCalled  bool
	probeErr    error
}

func (b *dialogConnectionProbeBackend) TestConnection(context.Context) error {
	b.probeCalled = true
	return b.probeErr
}

func (b *dialogConnectionProbeBackend) Root() string { return "d:/" }

func (b *dialogConnectionProbeBackend) Stat(context.Context, string) (RemoteEntry, error) {
	b.statCalled = true
	return RemoteEntry{}, nil
}

type dialogRootStatBackend struct {
	Backend
	location string
	err      error
}

func (*dialogRootStatBackend) Root() string { return "fallback-root" }

func (b *dialogRootStatBackend) Stat(_ context.Context, location string) (RemoteEntry, error) {
	b.location = location
	return RemoteEntry{}, b.err
}

func TestCloudProfileConnectionTestUsesProviderProbe(t *testing.T) {
	wantErr := errors.New("probe failed")
	backend := &dialogConnectionProbeBackend{probeErr: wantErr}
	if err := testCloudBackend(context.Background(), backend); !errors.Is(err, wantErr) {
		t.Fatalf("testCloudBackend error = %v, want %v", err, wantErr)
	}
	if !backend.probeCalled {
		t.Fatal("provider connection probe was not called")
	}
	if backend.statCalled {
		t.Fatal("root Stat was called despite the provider connection probe")
	}
}

func TestCloudProfileConnectionTestFallsBackToRootStat(t *testing.T) {
	wantErr := errors.New("stat failed")
	backend := &dialogRootStatBackend{err: wantErr}
	if err := testCloudBackend(context.Background(), backend); !errors.Is(err, wantErr) {
		t.Fatalf("testCloudBackend error = %v, want %v", err, wantErr)
	}
	if backend.location != backend.Root() {
		t.Fatalf("Stat location = %q, want root %q", backend.location, backend.Root())
	}
}

func newS3ProfileDialogTestHarness(t *testing.T, existing *Connection) *cloudProfileDialog {
	t.Helper()
	dialog := vtui.NewDialog(1, 1, 78, 24, " CloudFox: Amazon S3 / S3-compatible ")
	plugin := NewPlugin(Options{
		ConfigDir: t.TempDir(),
		Factories: []BackendFactory{&S3Factory{}},
		Keyring:   &memorySecretStore{},
		Vault:     &memorySecretStore{},
	})
	t.Cleanup(func() { _ = plugin.Close() })
	d := &cloudProfileDialog{
		plugin: plugin, provider: ProviderS3, original: existing, dialog: dialog,
		fields: make(map[string]*vtui.Edit), combos: make(map[string]*vtui.ComboBox), checks: make(map[string]*vtui.Checkbox),
		fieldX: dialog.X1 + 23, fieldWidth: 51, row: dialog.Y1 + 2,
	}
	d.name = d.addEdit("name", "&Name:", "S3 test", false)
	d.buildS3Fields(existing)
	return d
}

func TestS3ProfileTypingAnyStaticCredentialSelectsStaticAuthentication(t *testing.T) {
	for _, field := range []string{"access_key_id", "secret_access_key", "session_token"} {
		field := field
		t.Run(field, func(t *testing.T) {
			d := newS3ProfileDialogTestHarness(t, nil)
			// The failure is most visible with the default chain, but typing an
			// inline credential must be authoritative even if another external
			// source was selected before the user began typing.
			d.combos["auth"].Edit.SetText("profile")
			d.fields[field].InsertString("typed-value")
			if got := d.comboValue("auth"); got != "static" {
				t.Fatalf("authentication after typing %s = %q, want static", field, got)
			}
		})
	}
}

func TestS3ProfileSnapshotDefensivelyKeepsTypedStaticCredentials(t *testing.T) {
	for _, field := range []string{"access_key_id", "secret_access_key", "session_token"} {
		field := field
		t.Run(field, func(t *testing.T) {
			d := newS3ProfileDialogTestHarness(t, nil)
			d.combos["auth"].Edit.SetText("default")
			// SetText deliberately bypasses OnTextChange. connectionSnapshot is
			// the final trust boundary and must still prevent inline credentials
			// from being discarded before S3Factory.Open.
			d.fields[field].SetText("typed-value")

			connection, err := d.connectionSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			var settings S3Settings
			if err := json.Unmarshal(connection.Settings, &settings); err != nil {
				t.Fatal(err)
			}
			if settings.Auth != "static" {
				t.Fatalf("snapshot authentication with typed %s = %q, want static", field, settings.Auth)
			}

			values, changed := d.typedSecrets()
			if !changed || values[field] != "typed-value" {
				t.Fatalf("typed secrets before sanitization = %#v, changed=%v", values, changed)
			}
			sanitizeSecretsForConnection(connection, values)
			if values[field] != "typed-value" {
				t.Fatalf("typed %s was discarded after snapshot sanitization: %#v", field, values)
			}
		})
	}
}

func TestS3ProfileDefaultAuthenticationKeepsCompleteTypedAccessKeyPair(t *testing.T) {
	d := newS3ProfileDialogTestHarness(t, nil)
	if got := d.comboValue("auth"); got != "default" {
		t.Fatalf("initial authentication = %q, want default", got)
	}
	// Reproduce the original failure without relying on UI callbacks: both
	// credentials were present on screen while the combo still said default.
	d.fields["access_key_id"].SetText("AKIA-typed")
	d.fields["secret_access_key"].SetText("typed-secret")

	connection, err := d.connectionSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	values, changed := d.typedSecrets()
	if !changed {
		t.Fatal("typed access-key pair was not detected")
	}
	sanitizeSecretsForConnection(connection, values)
	if got := values["access_key_id"]; got != "AKIA-typed" {
		t.Fatalf("access key after sanitization = %q", got)
	}
	if got := values["secret_access_key"]; got != "typed-secret" {
		t.Fatalf("secret key after sanitization = %q", got)
	}
	if err := validateRequiredSecrets(connection, values); err != nil {
		t.Fatalf("complete typed access-key pair was not usable as static authentication: %v", err)
	}
}

func TestS3ProfileFieldsExplainOptionalBucketAndFollowDialogTheme(t *testing.T) {
	indices := []int{
		vtui.ColDialogText,
		vtui.ColDialogEdit,
		vtui.ColDialogSelectedButton,
		vtui.ColDialogHighlightText,
		vtui.ColDialogHighlightSelectedButton,
		vtui.ColDialogBox,
	}
	original := make(map[int]uint64, len(indices))
	for _, index := range indices {
		original[index] = vtui.Palette[index]
	}
	t.Cleanup(func() {
		for index, attr := range original {
			vtui.Palette[index] = attr
		}
	})

	dialog := vtui.NewDialog(1, 1, 78, 24, " CloudFox: Amazon S3 / S3-compatible ")
	d := &cloudProfileDialog{
		dialog: dialog,
		fields: make(map[string]*vtui.Edit), combos: make(map[string]*vtui.ComboBox), checks: make(map[string]*vtui.Checkbox),
		fieldX: dialog.X1 + 23, fieldWidth: 51, row: dialog.Y1 + 2,
	}
	d.buildS3Fields(nil)

	var bucketLabel, discoveryHint *vtui.Text
	for _, child := range dialog.GetChildren() {
		text, ok := child.(*vtui.Text)
		if !ok {
			continue
		}
		switch text.GetText() {
		case "&Bucket (optional):":
			bucketLabel = text
		case s3BucketDiscoveryHint:
			discoveryHint = text
		}
	}
	if bucketLabel == nil {
		t.Fatal("S3 dialog does not label Bucket as optional")
	}
	if discoveryHint == nil {
		t.Fatal("S3 dialog does not explain ListBuckets permission and manual fallback")
	}
	if discoveryHint.X2 > dialog.X2-2 {
		t.Fatalf("S3 discovery hint exceeds dialog content width: x2=%d, limit=%d", discoveryHint.X2, dialog.X2-2)
	}

	bucketEdit := d.fields["bucket"]
	pathStyle := d.checks["path_style"]
	if bucketEdit == nil || pathStyle == nil {
		t.Fatal("S3 dialog did not construct expected controls")
	}
	dialog.SetFocusedItem(pathStyle)
	dialog.SetFocus(true)

	renderAndCheck := func(name string, values map[int]uint64) {
		t.Helper()
		for index, attr := range values {
			vtui.Palette[index] = attr
		}
		screen := vtui.NewSilentScreenBuf()
		screen.AllocBuf(90, 28)
		dialog.Show(screen)

		if got := screen.GetCell(bucketLabel.X1+1, bucketLabel.Y1).Attributes; got != values[vtui.ColDialogText] {
			t.Fatalf("%s bucket label attr = %#x, want %#x", name, got, values[vtui.ColDialogText])
		}
		if got := screen.GetCell(bucketLabel.X1, bucketLabel.Y1).Attributes; got != values[vtui.ColDialogHighlightText] {
			t.Fatalf("%s bucket label hotkey attr = %#x, want %#x", name, got, values[vtui.ColDialogHighlightText])
		}
		if got := screen.GetCell(discoveryHint.X1, discoveryHint.Y1).Attributes; got != values[vtui.ColDialogText] {
			t.Fatalf("%s discovery hint attr = %#x, want %#x", name, got, values[vtui.ColDialogText])
		}
		if got := screen.GetCell(bucketEdit.X1, bucketEdit.Y1).Attributes; got != values[vtui.ColDialogEdit] {
			t.Fatalf("%s bucket edit attr = %#x, want %#x", name, got, values[vtui.ColDialogEdit])
		}
		if got := screen.GetCell(pathStyle.X1, pathStyle.Y1).Attributes; got != values[vtui.ColDialogSelectedButton] {
			t.Fatalf("%s focused checkbox attr = %#x, want %#x", name, got, values[vtui.ColDialogSelectedButton])
		}
		// Checkbox prefix is four cells; the hotkey is the fifth rune in
		// "Use path-style addressing".
		if got := screen.GetCell(pathStyle.X1+8, pathStyle.Y1).Attributes; got != values[vtui.ColDialogHighlightSelectedButton] {
			t.Fatalf("%s focused checkbox hotkey attr = %#x, want %#x", name, got, values[vtui.ColDialogHighlightSelectedButton])
		}
		if got := screen.GetCell(dialog.X1, dialog.Y1+1).Attributes; got != values[vtui.ColDialogBox] {
			t.Fatalf("%s dialog border attr = %#x, want %#x", name, got, values[vtui.ColDialogBox])
		}
	}

	first := map[int]uint64{
		vtui.ColDialogText:                    vtui.SetRGBBoth(0, 0x101112, 0x202122),
		vtui.ColDialogEdit:                    vtui.SetRGBBoth(0, 0x303132, 0x404142),
		vtui.ColDialogSelectedButton:          vtui.SetRGBBoth(0, 0x505152, 0x606162),
		vtui.ColDialogHighlightText:           vtui.SetRGBBoth(0, 0x707172, 0x808182),
		vtui.ColDialogHighlightSelectedButton: vtui.SetRGBBoth(0, 0x909192, 0xa0a1a2),
		vtui.ColDialogBox:                     vtui.SetRGBBoth(0, 0xb0b1b2, 0xc0c1c2),
	}
	renderAndCheck("initial theme", first)

	second := map[int]uint64{
		vtui.ColDialogText:                    vtui.SetRGBBoth(0, 0x111213, 0x212223),
		vtui.ColDialogEdit:                    vtui.SetRGBBoth(0, 0x313233, 0x414243),
		vtui.ColDialogSelectedButton:          vtui.SetRGBBoth(0, 0x515253, 0x616263),
		vtui.ColDialogHighlightText:           vtui.SetRGBBoth(0, 0x717273, 0x818283),
		vtui.ColDialogHighlightSelectedButton: vtui.SetRGBBoth(0, 0x919293, 0xa1a2a3),
		vtui.ColDialogBox:                     vtui.SetRGBBoth(0, 0xb1b2b3, 0xc1c2c3),
	}
	renderAndCheck("updated theme", second)
}

var _ Backend = (*dialogConnectionProbeBackend)(nil)
var _ Backend = (*dialogRootStatBackend)(nil)
