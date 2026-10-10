//go:build lite

package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unxed/f4/internal/update"
)

func TestUpdaterLiteSelectsOnlyLiteAssets(t *testing.T) {
	oldAPI, oldOS, oldArch := update.APIURL, update.CurrentOS, update.CurrentArch
	t.Cleanup(func() {
		update.APIURL, update.CurrentOS, update.CurrentArch = oldAPI, oldOS, oldArch
	})
	update.CurrentOS, update.CurrentArch = "linux", "amd64"
	regular := update.Asset{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.invalid/regular"}
	lite := update.Asset{Name: "f4-lite-linux-amd64.tar.gz", BrowserDownloadURL: "https://example.invalid/lite"}
	for _, tc := range []struct {
		name    string
		assets  []update.Asset
		wantURL string
	}{
		{name: "regular listed first", assets: []update.Asset{regular, lite}, wantURL: lite.BrowserDownloadURL},
		{name: "lite listed first", assets: []update.Asset{lite, regular}, wantURL: lite.BrowserDownloadURL},
		{name: "regular only is unavailable", assets: []update.Asset{regular}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewEncoder(w).Encode(update.Release{TagName: "v100.0.0", Assets: tc.assets}); err != nil {
					t.Errorf("encode release: %v", err)
				}
			}))
			defer server.Close()
			update.APIURL = server.URL
			candidate, err := update.Check(t.Context(), update.Settings{Channel: update.ChannelStable}, currentBuild())
			if tc.wantURL == "" {
				if err == nil || candidate.NeedsUpdate || candidate.DownloadURL != "" {
					t.Fatalf("regular-only release offered to lite: %#v, %v", candidate, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if candidate.DownloadURL != tc.wantURL || !candidate.NeedsUpdate || candidate.ArchiveKind != "targz" {
				t.Fatalf("lite update candidate = %#v, want lite tar.gz update", candidate)
			}
		})
	}
	t.Log("[FIX:lite-app] updater retains lite edition and rejects regular-only releases")
}
