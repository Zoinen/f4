package app

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestValidShareDisplayTextRejectsUnsafeRunesAndHonorsLimit(t *testing.T) {
	joinControl := "\u200d"
	tests := []struct {
		name  string
		value string
		limit int
		want  bool
	}{
		{name: "plain", value: "shared item", limit: 20, want: true},
		{name: "at limit", value: "1234", limit: 4, want: true},
		{name: "over limit", value: "12345", limit: 4, want: false},
		{name: "control", value: "line\nfeed", limit: 20, want: false},
		{name: "line separator", value: "line\u2028separator", limit: 20, want: false},
		{name: "format control", value: "hidden\u200btext", limit: 20, want: false},
		{name: "join control", value: "family" + joinControl + "name", limit: 20, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validShareDisplayText(tc.value, tc.limit); got != tc.want {
				t.Fatalf("validShareDisplayText(%q, %d) = %v, want %v", tc.value, tc.limit, got, tc.want)
			}
		})
	}
}

func TestShareLabelsCoverRolesAndExpirationUnits(t *testing.T) {
	roles := []struct {
		role vfs.ShareRole
		want string
	}{
		{vfs.ShareRoleViewer, i18n.Msg("Share.Role.Viewer")},
		{vfs.ShareRoleCommenter, i18n.Msg("Share.Role.Commenter")},
		{vfs.ShareRoleEditor, i18n.Msg("Share.Role.Editor")},
		{vfs.ShareRoleUploader, i18n.Msg("Share.Role.Uploader")},
		{vfs.ShareRoleServerControlled, i18n.Msg("Share.Role.ServerControlled")},
		{vfs.ShareRole(0), i18n.Msg("Share.NotAvailable")},
	}
	for _, tc := range roles {
		if got := shareRoleLabel(tc.role); got != tc.want {
			t.Errorf("shareRoleLabel(%d) = %q, want %q", tc.role, got, tc.want)
		}
	}

	expirations := []struct {
		duration time.Duration
		want     string
	}{
		{0, i18n.Msg("Share.Expiration.Never")},
		{2 * 24 * time.Hour, "2"},
		{3 * time.Hour, "3"},
		{15 * time.Minute, "15"},
	}
	for _, tc := range expirations {
		got := shareExpirationLabel(tc.duration)
		if tc.want == i18n.Msg("Share.Expiration.Never") {
			if got != tc.want {
				t.Errorf("shareExpirationLabel(%s) = %q, want %q", tc.duration, got, tc.want)
			}
			continue
		}
		if !strings.Contains(got, tc.want) {
			t.Errorf("shareExpirationLabel(%s) = %q, want text %q", tc.duration, got, tc.want)
		}
	}
}

func TestShareNoticeCoversProviderSpecificGuidance(t *testing.T) {
	tests := []struct {
		name string
		info vfs.ShareLinkInfo
		want string
	}{
		{name: "webdav", info: vfs.ShareLinkInfo{Provider: "WebDAV"}, want: i18n.Msg("Share.Notice.WebDAV")},
		{name: "s3", info: vfs.ShareLinkInfo{Provider: "Amazon S3"}, want: i18n.Msg("Share.Notice.S3")},
		{name: "yandex", info: vfs.ShareLinkInfo{Provider: "Yandex.Disk"}, want: i18n.Msg("Share.Notice.Yandex")},
		{name: "custom", info: vfs.ShareLinkInfo{Provider: "Other", Notice: "provider-specific note"}, want: "provider-specific note"},
		{name: "default public", info: vfs.ShareLinkInfo{Provider: "Other"}, want: i18n.Msg("Share.Notice.Public")},
	}
	for _, tc := range tests {
		if got := shareNotice(tc.info); got != tc.want {
			t.Errorf("%s: shareNotice() = %q, want %q", tc.name, got, tc.want)
		}
	}

	google := vfs.ShareLinkInfo{
		Provider:                     "Google Drive",
		Notice:                       "They do not allow changes",
		UnmanagedPublicAccess:        true,
		LinkDiscoverabilityInherited: true,
		LinkInherited:                true,
	}
	got := shareNotice(google)
	for _, want := range []string{
		i18n.Msg("Share.Notice.GooglePublished"),
		i18n.Msg("Share.Notice.Google"),
		i18n.Msg("Share.Notice.GoogleInheritedDiscoverable"),
		i18n.Msg("Share.Notice.GoogleInherited"),
		i18n.Msg("Share.Notice.GoogleReadOnly"),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Google notice %q does not contain %q", got, want)
		}
	}

	for name, info := range map[string]vfs.ShareLinkInfo{
		"discoverable": {Provider: "Google", LinkDiscoverable: true},
		"read-write":   {Provider: "Google"},
	} {
		text := shareNotice(info)
		if name == "discoverable" && !strings.Contains(text, i18n.Msg("Share.Notice.GoogleDiscoverable")) {
			t.Errorf("discoverable Google notice = %q", text)
		}
		if name == "read-write" && strings.Contains(text, i18n.Msg("Share.Notice.GoogleReadOnly")) {
			t.Errorf("read-write Google notice unexpectedly says read-only: %q", text)
		}
	}
}

func TestFormattedShareNoticeKeepsDialogHeightBounded(t *testing.T) {
	info := vfs.ShareLinkInfo{Provider: "Other", Notice: strings.Repeat("long notice word ", 80)}
	lines := formattedShareNotice(info)
	if len(lines) != 3 {
		t.Fatalf("formattedShareNotice returned %d lines, want 3", len(lines))
	}
	for _, line := range lines {
		if utf8.RuneCountInString(line) > 72 {
			t.Errorf("notice line has %d runes, want at most 72: %q", utf8.RuneCountInString(line), line)
		}
	}
}

func TestSetShareComboItemsClampsSelectionAndHandlesNoop(t *testing.T) {
	combo := vtui.NewComboBox(0, 0, 20, []string{"old"})
	combo.Edit.SetText("old")
	setShareComboItems(nil, []string{"ignored"}, 0)
	setShareComboItems(combo, nil, 0)
	if len(combo.Menu.Items) != 1 || combo.Edit.GetText() != "old" {
		t.Fatalf("empty update changed combo: items=%d text=%q", len(combo.Menu.Items), combo.Edit.GetText())
	}

	setShareComboItems(combo, []string{"viewer", "editor"}, 99)
	if combo.Menu.SelectPos != 0 || combo.Edit.GetText() != "viewer" || len(combo.Menu.Items) != 2 {
		t.Fatalf("out-of-range selection: pos=%d text=%q items=%d", combo.Menu.SelectPos, combo.Edit.GetText(), len(combo.Menu.Items))
	}
	setShareComboItems(combo, []string{"viewer", "editor"}, 1)
	if combo.Menu.SelectPos != 1 || combo.Edit.GetText() != "editor" {
		t.Fatalf("valid selection: pos=%d text=%q", combo.Menu.SelectPos, combo.Edit.GetText())
	}
}

func TestShareLinkCopyableAtChecksURLAndExpiry(t *testing.T) {
	now := time.Unix(100, 0)
	cases := []struct {
		name string
		link *vfs.ShareLink
		want bool
	}{
		{name: "nil", want: false},
		{name: "missing URL", link: &vfs.ShareLink{}, want: false},
		{name: "permanent", link: &vfs.ShareLink{URL: "https://share.example/item"}, want: true},
		{name: "active", link: &vfs.ShareLink{URL: "https://share.example/item", ExpiresAt: now.Add(time.Minute)}, want: true},
		{name: "expired", link: &vfs.ShareLink{URL: "https://share.example/item", ExpiresAt: now}, want: false},
	}
	for _, tc := range cases {
		if got := shareLinkCopyableAt(tc.link, now); got != tc.want {
			t.Errorf("%s: shareLinkCopyableAt() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
