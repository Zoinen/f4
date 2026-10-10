package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unxed/f4/internal/netproxy"
)

// #1218: the releases as GitHub served them on 2026-09-28. The stable release
// was published before the nightly's commit.
const (
	issue1218StablePublished = "2026-09-26T23:20:10Z"
	issue1218NightlyBody     = "Automated pre-release build of the latest main branch. Commit: 321a594. Built on: 2026-09-28 02:02:43"
	issue1218NightlyAsset    = "2026-09-28T02:20:17Z"
)

var (
	// The nightly build of 321a594, the newest one on 2026-09-28.
	issue1218NightlyBuild = Build{Version: "321a594", TimeText: "2026-09-28T02:02:43Z"}
	// The nightly build Alexey5112 was running, 6a96e1d of 2026-09-26.
	issue1218OldNightlyBuild = Build{Version: "6a96e1d", TimeText: "2026-09-26T07:00:39Z"}
)

func serveIssue1218Releases(t *testing.T) {
	t.Helper()
	oldAPI, oldOS, oldArch, oldProxy := APIURL, CurrentOS, CurrentArch, netproxy.Global()
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch = oldAPI, oldOS, oldArch
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var release Release
		switch r.URL.Path {
		case "/latest":
			release = Release{
				TagName:     "v0.3.0-beta",
				PublishedAt: issue1218StablePublished,
				Assets:      []Asset{{Name: testLinuxAssetName(), BrowserDownloadURL: "https://example/stable", UpdatedAt: "2026-09-26T23:10:00Z"}},
			}
		case "/tags/nightly":
			release = Release{
				TagName:     "nightly",
				PublishedAt: "2026-09-28T02:20:25Z",
				Body:        issue1218NightlyBody,
				Assets:      []Asset{{Name: testLinuxAssetName(), BrowserDownloadURL: "https://example/nightly", UpdatedAt: issue1218NightlyAsset}},
			}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(release)
	}))
	t.Cleanup(server.Close)
	APIURL = server.URL
}

func TestIssue1218StableOfferedAfterMovingFromNightly(t *testing.T) {
	serveIssue1218Releases(t)

	cases := []struct {
		name      string
		cfg       Settings
		build     Build
		wantOffer bool
		wantOlder bool
	}{
		{
			// The nightly is newer than the release, so a plain stable check
			// finds nothing: someone who downloaded a nightly by hand and
			// never touched the channel is not nagged to go back.
			name:  "nightly build, stable channel, no switch",
			cfg:   Settings{Channel: ChannelStable},
			build: issue1218NightlyBuild,
		},
		{
			name:      "nightly build, channel just switched to stable",
			cfg:       Settings{Channel: ChannelStable, SwitchedChannel: true},
			build:     issue1218NightlyBuild,
			wantOffer: true,
			wantOlder: true,
		},
		{
			// The switch was saved earlier (settings dialog OK, settings
			// center); the updater had last installed a nightly.
			name:      "nightly build, stable channel, updater last installed a nightly",
			cfg:       Settings{Channel: ChannelStable, LastVersion: issue1218NightlyAsset},
			build:     issue1218NightlyBuild,
			wantOffer: true,
			wantOlder: true,
		},
		{
			// Alexey5112's case: his nightly predates the release, which the
			// dates already offered.
			name:      "older nightly build, channel switched to stable",
			cfg:       Settings{Channel: ChannelStable, SwitchedChannel: true, LastVersion: issue1218NightlyAsset},
			build:     issue1218OldNightlyBuild,
			wantOffer: true,
		},
		{
			name:  "the stable release itself",
			cfg:   Settings{Channel: ChannelStable, SwitchedChannel: true, LastVersion: issue1218NightlyAsset},
			build: Build{Version: "v0.3.0-beta", TimeText: "2026-09-26T22:41:49Z"},
		},
		{
			name:  "stable already installed by the updater",
			cfg:   Settings{Channel: ChannelStable, LastVersion: "v0.3.0-beta"},
			build: issue1218NightlyBuild,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cand, err := Check(context.Background(), tc.cfg, tc.build)
			if err != nil {
				t.Fatalf("Check() error: %v", err)
			}
			if cand.NeedsUpdate != tc.wantOffer || cand.OlderThanRunning != tc.wantOlder {
				t.Fatalf("Check() NeedsUpdate=%v OlderThanRunning=%v, want %v %v", cand.NeedsUpdate, cand.OlderThanRunning, tc.wantOffer, tc.wantOlder)
			}
			if cand.UpdateKey != "v0.3.0-beta" || cand.DownloadURL != "https://example/stable" {
				t.Fatalf("Check() = %+v, want the stable release", cand)
			}
		})
	}
}

// A release build is compared by tag alone: it has no nightly to leave.
func TestIssue1218ReleaseBuildIsNotLeavingNightly(t *testing.T) {
	if leavingNightly(Settings{SwitchedChannel: true, LastVersion: issue1218NightlyAsset}, Build{Version: "v0.3.0", IsRelease: true}) {
		t.Fatal("a release build must not count as leaving nightly")
	}
	if leavingNightly(Settings{LastVersion: "v0.2.0-beta"}, issue1218NightlyBuild) {
		t.Fatal("a release tag in LastVersion is not a nightly install")
	}
	if leavingNightly(Settings{}, issue1218NightlyBuild) {
		t.Fatal("nothing says the user came from nightly")
	}
}

// The nightly channel used to trust LastVersion alone: once an install of the
// current nightly finished, a binary that still ran the older build heard
// "Already up to date" (#1218: 6a96e1d of 2026-09-26 against the nightly of
// 2026-09-28).
func TestIssue1218NightlyNotUpToDateWhileOlderBuildRuns(t *testing.T) {
	serveIssue1218Releases(t)

	cfg := Settings{Channel: ChannelNightly, LastVersion: issue1218NightlyAsset}
	cand, err := Check(context.Background(), cfg, issue1218OldNightlyBuild)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if !cand.NeedsUpdate {
		t.Fatalf("older running build told it is up to date: %+v", cand)
	}

	cand, err = Check(context.Background(), cfg, issue1218NightlyBuild)
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if cand.NeedsUpdate {
		t.Fatalf("the installed nightly itself offered again: %+v", cand)
	}
	// Named by commit and build time, like F1 names the running build.
	if want := "Nightly (321a594 [" + FormatBuildTime("2026-09-28 02:02:43") + "])"; cand.DisplayVersion != want {
		t.Fatalf("DisplayVersion = %q, want %q", cand.DisplayVersion, want)
	}

	// A newer local build on the nightly channel is not sent back.
	cand, err = Check(context.Background(), cfg, Build{Version: "local", TimeText: "2026-09-29T00:00:00Z"})
	if err != nil {
		t.Fatalf("Check() error: %v", err)
	}
	if cand.NeedsUpdate {
		t.Fatalf("newer local build offered the nightly: %+v", cand)
	}
}

func TestIssue1218PlainNightlyReleaseBody(t *testing.T) {
	commit, builtOn := commitInfoFromReleaseBody(issue1218NightlyBody)
	if commit != "321a594" || builtOn != "2026-09-28 02:02:43" {
		t.Fatalf("commitInfoFromReleaseBody() = %q, %q", commit, builtOn)
	}
	if got := parseBuildTime(builtOn); !got.Equal(parseBuildTime("2026-09-28T02:02:43Z")) {
		t.Fatalf("parseBuildTime(%q) = %v", builtOn, got)
	}
}

// `f4 --update stable` on a nightly setup goes on to install the release; it
// stops here only because the install target is unavailable. Without the
// switch the same build is up to date.
func TestIssue1218RunCLISwitchToStableInstallsRelease(t *testing.T) {
	serveIssue1218Releases(t)
	oldExecutable := Executable
	t.Cleanup(func() { Executable = oldExecutable })
	Executable = func() (string, error) { return "", errors.New("executable unavailable") }

	var saved Settings
	if got := RunCLI("stable", Settings{Channel: ChannelNightly}, issue1218NightlyBuild, func(s Settings) { saved = s }); got != 1 {
		t.Fatalf("RunCLI(stable from nightly) = %d, want 1 (install attempted)", got)
	}
	if saved.Channel != ChannelStable {
		t.Fatalf("channel not saved: %+v", saved)
	}
	if saved.SwitchedChannel {
		t.Fatalf("the one-off switch flag leaked into the saved settings: %+v", saved)
	}

	if got := RunCLI("stable", Settings{Channel: ChannelStable}, issue1218NightlyBuild, func(Settings) {}); got != 0 {
		t.Fatalf("RunCLI(stable, no switch) = %d, want 0 (already up to date)", got)
	}
}
