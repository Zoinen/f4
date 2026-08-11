package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type shareDialogTestProvider struct{}

func (shareDialogTestProvider) ShareLinkInfo(context.Context, string) (vfs.ShareLinkInfo, error) {
	return vfs.ShareLinkInfo{}, nil
}

func (shareDialogTestProvider) CreateShareLink(context.Context, string, vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	return vfs.ShareLink{}, nil
}

func (shareDialogTestProvider) RevokeShareLink(context.Context, string) error { return nil }

func TestShareDialogExistingLinkSelectsItsRoleAndDefaultsToCopy(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider:  "Google Drive",
		ItemName:  "report.docx",
		Roles:     []vfs.ShareRole{vfs.ShareRoleViewer, vfs.ShareRoleCommenter, vfs.ShareRoleEditor},
		CanCreate: true,
		CanRevoke: true,
		Link: &vfs.ShareLink{
			URL:       "https://share.invalid/editor-link",
			Role:      vfs.ShareRoleEditor,
			Revocable: true,
		},
	})
	defer d.dialog.Close()

	if got := d.role.Menu.SelectPos; got != 2 {
		t.Fatalf("selected role index = %d, want Editor at 2", got)
	}
	request, ok := d.request()
	if !ok || request.Role != vfs.ShareRoleEditor {
		t.Fatalf("request = %#v, %v; want existing Editor role", request, ok)
	}
	if !d.copy.IsDefault || d.create.IsDefault {
		t.Fatalf("default buttons: create=%v copy=%v", d.create.IsDefault, d.copy.IsDefault)
	}
	if focused := d.dialog.GetFocusedItem(); focused != d.copy {
		t.Fatalf("focused item = %T, want Copy button", focused)
	}
	if d.copy.IsDisabled() || d.create.IsDisabled() || d.revoke.IsDisabled() {
		t.Fatalf("active modifiable link buttons: create=%v copy=%v revoke=%v",
			d.create.IsDisabled(), d.copy.IsDisabled(), d.revoke.IsDisabled())
	}
}

func TestShareDialogCannotCreateWithoutAProviderRole(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider:  "Broken provider",
		ItemName:  "file.txt",
		CanCreate: true,
	})
	defer d.dialog.Close()

	if d.canCreate() {
		t.Fatal("dialog can create a link without a provider-supported role")
	}
	if !d.create.IsDisabled() || d.create.IsDefault {
		t.Fatalf("Create button disabled=%v default=%v", d.create.IsDisabled(), d.create.IsDefault)
	}
	if _, ok := d.request(); ok {
		t.Fatal("dialog produced a request without a provider-supported role")
	}
	focused, ok := d.dialog.GetFocusedItem().(*vtui.Button)
	if !ok || focused.GetCaption() != Msg("Share.Close") {
		t.Fatalf("focused item = %#v, want Close button", d.dialog.GetFocusedItem())
	}
}

func TestShareDialogPrivateShareableItemDefaultsToCreate(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider:  "Yandex.Disk",
		ItemName:  "folder",
		Roles:     []vfs.ShareRole{vfs.ShareRoleViewer},
		CanCreate: true,
	})
	defer d.dialog.Close()

	if d.create.IsDisabled() || !d.create.IsDefault || d.copy.IsDefault {
		t.Fatalf("private item defaults: create disabled=%v create default=%v copy default=%v",
			d.create.IsDisabled(), d.create.IsDefault, d.copy.IsDefault)
	}
	if focused := d.dialog.GetFocusedItem(); focused != d.create {
		t.Fatalf("focused item = %T, want Create button", focused)
	}
}

func TestShareDialogShowsMaximumExpiryHonestly(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	expiresAt := time.Now().Add(time.Hour).Round(time.Second)
	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider: "Amazon S3",
		ItemName: "backup.bin",
		Roles:    []vfs.ShareRole{vfs.ShareRoleViewer},
		Link: &vfs.ShareLink{
			URL:                "https://bucket.example/backup.bin?signature=redacted",
			Role:               vfs.ShareRoleViewer,
			ExpiresAt:          expiresAt,
			ExpiresAtIsMaximum: true,
		},
	})
	defer d.dialog.Close()

	want := fmt.Sprintf(Msg("Share.ActiveNoLaterThan"), expiresAt.Local().Format(time.RFC822))
	want = vtui.TruncateMiddle(want, 72)
	if got := d.status.GetText(); got != want {
		t.Fatalf("status = %q, want %q", got, want)
	}
}

func TestValidateShareLinkInfoRejectsUntrustedProviderData(t *testing.T) {
	valid := vfs.ShareLinkInfo{
		Provider:          "Provider",
		ItemName:          "file.txt",
		Roles:             []vfs.ShareRole{vfs.ShareRoleViewer},
		ExpirationOptions: []time.Duration{0, time.Hour},
		DefaultExpiration: time.Hour,
		Link: &vfs.ShareLink{
			URL:  "https://share.example/item",
			Role: vfs.ShareRoleViewer,
		},
	}

	tests := map[string]func(*vfs.ShareLinkInfo){
		"control character": func(info *vfs.ShareLinkInfo) { info.Provider = "bad\nprovider" },
		"invalid role":      func(info *vfs.ShareLinkInfo) { info.Roles[0] = vfs.ShareRole(99) },
		"unsafe URL":        func(info *vfs.ShareLinkInfo) { info.Link.URL = "https://user:secret@share.example/item" },
		"hidden active role": func(info *vfs.ShareLinkInfo) {
			info.Link.Role = vfs.ShareRoleEditor
		},
		"unsupported default expiration": func(info *vfs.ShareLinkInfo) {
			info.DefaultExpiration = 2 * time.Hour
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			info := valid
			info.Roles = append([]vfs.ShareRole(nil), valid.Roles...)
			info.ExpirationOptions = append([]time.Duration(nil), valid.ExpirationOptions...)
			link := *valid.Link
			info.Link = &link
			mutate(&info)
			if err := validateShareLinkInfo(info); err == nil {
				t.Fatal("untrusted provider metadata was accepted")
			}
		})
	}
}

func TestSafeShareErrorMessageRedactsBearerURLsAndTokens(t *testing.T) {
	err := errors.New("request https://bucket.example/file?X-Amz-Credential=AKIA&X-Amz-Signature=secret failed; token=also-secret")
	message := safeShareErrorMessage(err)
	for _, secret := range []string{"bucket.example", "AKIA", "secret", "also-secret"} {
		if strings.Contains(message, secret) {
			t.Fatalf("sanitized error %q still contains %q", message, secret)
		}
	}
	if !strings.Contains(message, "[link redacted]") || !strings.Contains(message, "token=[redacted]") {
		t.Fatalf("sanitized error = %q", message)
	}

	long := safeShareErrorMessage(errors.New(strings.Repeat("я", 5000)))
	if got := len([]rune(long)); got != 4096 {
		t.Fatalf("sanitized long error has %d runes, want 4096", got)
	}
}

func TestShareDialogDoesNotClaimS3HasNoIssuedLinks(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider:          "Amazon S3",
		ItemName:          "object.bin",
		Roles:             []vfs.ShareRole{vfs.ShareRoleViewer},
		ExpirationOptions: []time.Duration{time.Hour},
		DefaultExpiration: time.Hour,
		CanCreate:         true,
		LinksUnenumerable: true,
	})
	defer d.dialog.Close()
	if got := d.link.GetText(); got != Msg("Share.LinkUnenumerable") {
		t.Fatalf("link text = %q", got)
	}
	if got := d.status.GetText(); got != vtui.TruncateMiddle(Msg("Share.UnenumerableStatus"), 72) {
		t.Fatalf("status = %q", got)
	}
}

func TestShareDialogDisablesExpiredAndUnknownStateActions(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider:  "Amazon S3",
		ItemName:  "object.bin",
		Roles:     []vfs.ShareRole{vfs.ShareRoleViewer},
		CanCreate: true,
		CanRevoke: true,
		Link: &vfs.ShareLink{
			URL:       "https://objects.example/object.bin?signature=expired",
			Role:      vfs.ShareRoleViewer,
			ExpiresAt: time.Now().Add(-time.Second),
			Revocable: true,
		},
	})
	defer d.dialog.Close()
	if !d.copy.IsDisabled() || d.status.GetText() != vtui.TruncateMiddle(Msg("Share.Expired"), 72) {
		t.Fatalf("expired link copy disabled=%v status=%q", d.copy.IsDisabled(), d.status.GetText())
	}

	d.info.Link.ExpiresAt = time.Now().Add(time.Hour)
	d.refresh("")
	if d.copy.IsDisabled() {
		t.Fatal("fresh link unexpectedly disabled before unknown state")
	}
	d.stateUnknown = true
	d.setBusy(false)
	if !d.create.IsDisabled() || !d.copy.IsDisabled() || !d.revoke.IsDisabled() {
		t.Fatalf("unknown state actions: create=%v copy=%v revoke=%v", d.create.IsDisabled(), d.copy.IsDisabled(), d.revoke.IsDisabled())
	}
}

func TestShareDialogUnexpectedCreatedAccessBlocksCopyButAllowsRevoke(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider:  "Google Drive",
		ItemName:  "report.docx",
		Roles:     []vfs.ShareRole{vfs.ShareRoleViewer, vfs.ShareRoleEditor},
		CanCreate: true,
		CanRevoke: true,
		Link: &vfs.ShareLink{
			URL: "https://share.example/unexpected-editor", Role: vfs.ShareRoleEditor, Revocable: true,
		},
	})
	defer d.dialog.Close()
	d.unexpectedAccess = true
	d.setBusy(false)
	if !d.create.IsDisabled() || !d.copy.IsDisabled() {
		t.Fatalf("unexpected access left create/copy enabled: create=%v copy=%v", d.create.IsDisabled(), d.copy.IsDisabled())
	}
	if d.revoke.IsDisabled() {
		t.Fatal("unexpected access disabled the only safe remediation: Revoke")
	}
}

func TestShareDialogAuthoritativeReconciliationRebuildsSelectors(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider:          "Google Drive",
		ItemName:          "report.docx",
		Roles:             []vfs.ShareRole{vfs.ShareRoleViewer, vfs.ShareRoleCommenter},
		ExpirationOptions: []time.Duration{time.Hour, 24 * time.Hour},
		DefaultExpiration: time.Hour,
		CanCreate:         true,
	})
	defer d.dialog.Close()

	d.applyAuthoritativeInfo(vfs.ShareLinkInfo{
		Provider:          "Google Drive",
		ItemName:          "report.docx",
		Roles:             []vfs.ShareRole{vfs.ShareRoleEditor},
		ExpirationOptions: []time.Duration{0},
		DefaultExpiration: 0,
		CanCreate:         false,
		CanRevoke:         false,
		LinkInherited:     true,
		Link: &vfs.ShareLink{
			URL: "https://share.example/inherited-editor", Role: vfs.ShareRoleEditor,
		},
	})
	d.setBusy(false)

	request, ok := d.request()
	if !ok || request.Role != vfs.ShareRoleEditor || request.ExpiresIn != 0 {
		t.Fatalf("request after authoritative refresh = %#v, %v", request, ok)
	}
	if len(d.role.Menu.Items) != 1 || d.role.Edit.GetText() != shareRoleLabel(vfs.ShareRoleEditor) {
		t.Fatalf("role selector was not rebuilt: items=%#v text=%q", d.role.Menu.Items, d.role.Edit.GetText())
	}
	if len(d.expiration.Menu.Items) != 1 || d.expiration.Edit.GetText() != shareExpirationLabel(0) {
		t.Fatalf("expiration selector was not rebuilt: items=%#v text=%q", d.expiration.Menu.Items, d.expiration.Edit.GetText())
	}
	if !d.create.IsDisabled() || !d.role.IsDisabled() || !d.expiration.IsDisabled() {
		t.Fatal("authoritative read-only capabilities left mutation selectors enabled")
	}
}

func TestShareDialogSerializesClipboardSoNewestLinkWins(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{}, 2)
	var (
		mu     sync.Mutex
		writes []string
	)
	d := &shareLinkDialog{setClipboard: func(link string) {
		if strings.Contains(link, "older") {
			close(started)
			<-release
		}
		mu.Lock()
		writes = append(writes, link)
		mu.Unlock()
		finished <- struct{}{}
	}}

	d.copyLinkToClipboard("https://share.example/older")
	<-started
	d.copyLinkToClipboard("https://share.example/newest")
	close(release)
	<-finished
	<-finished

	mu.Lock()
	defer mu.Unlock()
	if len(writes) != 2 || writes[len(writes)-1] != "https://share.example/newest" {
		t.Fatalf("clipboard writes = %#v, newest link must be final", writes)
	}
}

func TestShareDialogPresentsWebDAVAccessAsServerControlled(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", vfs.ShareLinkInfo{
		Provider: "Generic WebDAV",
		ItemName: "folder",
		Roles:    []vfs.ShareRole{vfs.ShareRoleServerControlled},
		Link: &vfs.ShareLink{
			URL: "http://dav.example/folder/", Role: vfs.ShareRoleServerControlled,
		},
	})
	defer d.dialog.Close()
	if got := d.expiration.Edit.GetText(); got != Msg("Share.Expiration.ServerControlled") {
		t.Fatalf("expiration = %q", got)
	}
	if got := d.status.GetText(); got != vtui.TruncateMiddle(Msg("Share.ServerControlledStatus"), 72) {
		t.Fatalf("status = %q", got)
	}
}

func TestShareDialogIgnoresStaleExpiryTimerAfterLinkReplacement(t *testing.T) {
	oldExpiry := time.Now().Add(-time.Second)
	newExpiry := time.Now().Add(time.Hour)
	d := &shareLinkDialog{
		info:             vfs.ShareLinkInfo{Link: &vfs.ShareLink{URL: "https://share.example/new", ExpiresAt: newExpiry}},
		expiryGeneration: 2,
	}
	if d.expiryTimerStillCurrent(1, "https://share.example/old", oldExpiry, time.Now()) {
		t.Fatal("stale timer matched a replacement link")
	}
	if d.expiryTimerStillCurrent(2, "https://share.example/new", newExpiry, time.Now()) {
		t.Fatal("future current link was treated as expired")
	}
	if !d.expiryTimerStillCurrent(2, "https://share.example/new", newExpiry, newExpiry) {
		t.Fatal("current timer did not match at expiry")
	}
}

func TestValidateShareLinkInfoRejectsContradictoryCapabilitiesAndUnsafeText(t *testing.T) {
	tests := []vfs.ShareLinkInfo{
		{
			Provider:  "Provider",
			Roles:     []vfs.ShareRole{vfs.ShareRoleViewer},
			CanRevoke: true,
			Link:      &vfs.ShareLink{URL: "https://share.example/item", Role: vfs.ShareRoleViewer},
		},
		{
			Provider: "Provider",
			Roles:    []vfs.ShareRole{vfs.ShareRoleViewer},
			Link: &vfs.ShareLink{
				URL: "https://share.example/item", Role: vfs.ShareRoleViewer, ExpiresAtIsMaximum: true,
			},
		},
		{Provider: "Provider\u202e", Roles: []vfs.ShareRole{vfs.ShareRoleViewer}},
		{Provider: "Provider", Notice: "debug URL https://signed.example/?token=secret", Roles: []vfs.ShareRole{vfs.ShareRoleViewer}},
	}
	for index, info := range tests {
		if err := validateShareLinkInfo(info); err == nil {
			t.Fatalf("case %d was accepted: %#v", index, info)
		}
	}
	if err := validateShareLinkInfo(vfs.ShareLinkInfo{
		Provider: "Google Drive",
		ItemName: "Family 👨‍👩‍👧‍👦.png",
		Notice:   "Shared family file 👨‍👩‍👧‍👦",
		Roles:    []vfs.ShareRole{vfs.ShareRoleViewer},
	}); err != nil {
		t.Fatalf("ordinary ZWJ display text rejected: %v", err)
	}
}

func TestGoogleShareNoticeDistinguishesDirectAndInheritedDiscoverability(t *testing.T) {
	info := vfs.ShareLinkInfo{
		Provider:         "Google Drive",
		LinkInherited:    true,
		LinkDiscoverable: true,
	}
	notice := shareNotice(info)
	for _, expected := range []string{Msg("Share.Notice.GoogleDiscoverable"), Msg("Share.Notice.GoogleInherited")} {
		if !strings.Contains(notice, expected) {
			t.Fatalf("notice %q does not contain %q", notice, expected)
		}
	}
	if strings.Contains(notice, Msg("Share.Notice.GoogleInheritedDiscoverable")) {
		t.Fatalf("direct discoverability was mislabeled as inherited: %q", notice)
	}
}

func TestGooglePublishedExposureNoticeIsVisibleAndActionable(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	info := vfs.ShareLinkInfo{
		Provider:              "Google Drive",
		ItemName:              "Published report",
		Roles:                 []vfs.ShareRole{vfs.ShareRoleViewer},
		CanCreate:             true,
		UnmanagedPublicAccess: true,
	}
	notice := shareNotice(info)
	if !strings.HasPrefix(notice, Msg("Share.Notice.GooglePublished")) {
		t.Fatalf("published remediation is not prominent: %q", notice)
	}
	d := showShareLinkDialog(nil, shareDialogTestProvider{}, "opaque-item", info)
	defer d.dialog.Close()
	var visible strings.Builder
	for _, line := range d.noticeLines {
		visible.WriteString(line.GetText())
		visible.WriteByte(' ')
	}
	wantRunes := []rune(Msg("Share.Notice.GooglePublished"))
	wantPrefix := string(wantRunes[:min(24, len(wantRunes))])
	if text := visible.String(); !strings.Contains(text, wantPrefix) {
		t.Fatalf("published remediation was hidden in dialog notice: %q", text)
	}
	if got := d.link.GetText(); got != Msg("Share.UnmanagedAccess") {
		t.Fatalf("published view exposure was presented as private: %q", got)
	}
	if got := d.status.GetText(); got != vtui.TruncateMiddle(Msg("Share.UnmanagedStatus"), 72) {
		t.Fatalf("published view status = %q", got)
	}
}

var _ vfs.ShareLinkProvider = shareDialogTestProvider{}
