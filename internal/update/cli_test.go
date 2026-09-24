package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/netproxy"
)

func TestParseUpdateChannelArg(t *testing.T) {
	cases := []struct {
		arg      string
		want     int
		explicit bool
		wantErr  bool
	}{
		{"", ChannelNightly, false, false}, // empty means the configured channel
		{"stable", ChannelStable, true, false},
		{"latest", ChannelStable, true, false},
		{"Nightly", ChannelNightly, true, false},
		{" nightly ", ChannelNightly, true, false},
		{"beta", 0, false, true},
	}
	for _, c := range cases {
		got, explicit, err := ParseChannelArg(c.arg, ChannelNightly)
		if (err != nil) != c.wantErr {
			t.Fatalf("%q: err = %v, wantErr %v", c.arg, err, c.wantErr)
		}
		if err != nil {
			continue
		}
		if got != c.want || explicit != c.explicit {
			t.Errorf("%q: got channel %d explicit %v, want %d %v", c.arg, got, explicit, c.want, c.explicit)
		}
	}
}

// The channel decides which endpoint is asked and how the build is named,
// which is the whole difference between --update nightly and --update stable.
func TestFetchUpdateCandidateFollowsChannel(t *testing.T) {
	origOS, origArch, origAPI := CurrentOS, CurrentArch, APIURL
	t.Cleanup(func() {
		CurrentOS, CurrentArch, APIURL = origOS, origArch, origAPI
	})
	CurrentOS, CurrentArch = "linux", "amd64"

	var asked string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		release := Release{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Body:        "**Commit:** `abc1234`\n**Built on:** `2030-01-01 00:00`",
			Assets: []Asset{{
				Name:               "f4-linux-amd64.tar.gz",
				BrowserDownloadURL: "http://mock/f4.tar.gz",
				UpdatedAt:          "2030-01-01T00:00:00Z",
			}},
		}
		if err := json.NewEncoder(w).Encode(release); err != nil {
			t.Errorf("encode release: %v", err)
		}
	}))
	defer ts.Close()
	APIURL = ts.URL + "/repos/unxed/f4/releases"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cand, err := Check(ctx, Settings{Channel: ChannelNightly}, Build{})
	if err != nil {
		t.Fatalf("nightly: %v", err)
	}
	if asked != "/repos/unxed/f4/releases/tags/nightly" {
		t.Errorf("nightly asked %q", asked)
	}
	// The build time renders in local time, so match on the commit instead.
	if !strings.HasPrefix(cand.DisplayVersion, "Nightly (abc1234 [") {
		t.Errorf("nightly display version %q", cand.DisplayVersion)
	}
	if cand.UpdateKey != "2030-01-01T00:00:00Z" || !cand.NeedsUpdate {
		t.Errorf("nightly key %q NeedsUpdate %v", cand.UpdateKey, cand.NeedsUpdate)
	}
	if cand.ArchiveKind != "targz" || cand.DownloadURL != "http://mock/f4.tar.gz" {
		t.Errorf("nightly asset %q %q", cand.ArchiveKind, cand.DownloadURL)
	}

	cand, err = Check(ctx, Settings{Channel: ChannelStable}, Build{})
	if err != nil {
		t.Fatalf("stable: %v", err)
	}
	if asked != "/repos/unxed/f4/releases/latest" {
		t.Errorf("stable asked %q", asked)
	}
	if cand.DisplayVersion != "v100.0.0" || cand.UpdateKey != "v100.0.0" {
		t.Errorf("stable version %q key %q", cand.DisplayVersion, cand.UpdateKey)
	}
}

// A 403 from the spent request limit has to explain itself rather than show a
// number: it is what anyone who checks for updates often runs into.
func TestFetchUpdateCandidateExplainsRateLimit(t *testing.T) {
	origAPI := APIURL
	t.Cleanup(func() { APIURL = origAPI })

	reset := time.Now().Add(42 * time.Minute)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	APIURL = ts.URL

	_, err := Check(context.Background(), Settings{Channel: ChannelStable}, Build{})
	if err == nil {
		t.Fatal("rate-limited check must fail")
	}
	if !strings.Contains(err.Error(), "60 per hour") || !strings.Contains(err.Error(), reset.Format("15:04")) {
		t.Errorf("unhelpful rate limit message: %q", err)
	}
}

func TestRunCLIRejectsUnknownChannel(t *testing.T) {
	if got := RunCLI("beta", Settings{Channel: ChannelStable}, Build{}, func(Settings) {}); got != 2 {
		t.Fatalf("RunCLI(unknown channel) = %d, want 2", got)
	}
}

func TestRunCLIReportsCheckFailure(t *testing.T) {
	oldAPI, oldProxy := APIURL, netproxy.Global()
	t.Cleanup(func() {
		APIURL = oldAPI
		netproxy.SetGlobal(oldProxy)
	})
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	APIURL = server.URL

	if got := RunCLI("", Settings{Channel: ChannelStable}, Build{}, func(Settings) {}); got != 1 {
		t.Fatalf("RunCLI(check failure) = %d, want 1", got)
	}
}

func TestRunCLIStopsWhenAlreadyUpToDate(t *testing.T) {
	oldAPI, oldOS, oldArch, oldProxy := APIURL, CurrentOS, CurrentArch, netproxy.Global()
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch = oldAPI, oldOS, oldArch
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v1.0.0",
			Assets:  []Asset{{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "unused"}},
		})
	}))
	defer server.Close()
	APIURL = server.URL

	saves := 0
	if got := RunCLI("", Settings{Channel: ChannelStable}, Build{Version: "v1.0.0", IsRelease: true}, func(Settings) { saves++ }); got != 0 {
		t.Fatalf("RunCLI(already current) = %d, want 0", got)
	}
	if saves != 0 {
		t.Fatalf("already-current run saved settings %d times, want 0", saves)
	}
}

func TestRunCLIChangesChannelBeforeTargetCheck(t *testing.T) {
	oldAPI, oldOS, oldArch, oldProxy, oldExecutable := APIURL, CurrentOS, CurrentArch, netproxy.Global(), Executable
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch, Executable = oldAPI, oldOS, oldArch, oldExecutable
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "nightly",
			Assets:  []Asset{{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "unused", UpdatedAt: "2026-09-18T00:00:00Z"}},
		})
	}))
	defer server.Close()
	APIURL = server.URL
	Executable = func() (string, error) { return "", errors.New("executable unavailable") }

	saved := Settings{}
	saves := 0
	if got := RunCLI("nightly", Settings{Channel: ChannelStable}, Build{}, func(s Settings) { saved, saves = s, saves+1 }); got != 1 {
		t.Fatalf("RunCLI(target failure) = %d, want 1", got)
	}
	if saves != 1 || saved.Channel != ChannelNightly {
		t.Fatalf("channel save = %+v, count %d; want nightly, once", saved, saves)
	}
}

func TestRunCLIFailsOnDownload(t *testing.T) {
	oldAPI, oldOS, oldArch, oldProxy, oldExecutable := APIURL, CurrentOS, CurrentArch, netproxy.Global(), Executable
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch, Executable = oldAPI, oldOS, oldArch, oldExecutable
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})
	dir := t.TempDir()
	exe := filepath.Join(dir, "f4")
	if err := os.WriteFile(exe, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	Executable = func() (string, error) { return exe, nil }
	archiveURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/archive" {
			http.Error(w, "download unavailable", http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v2.0.0",
			Assets:  []Asset{{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: archiveURL}},
		})
	}))
	defer server.Close()
	archiveURL = server.URL + "/archive"
	APIURL = server.URL

	if got := RunCLI("stable", Settings{Channel: ChannelStable}, Build{Version: "v1.0.0", IsRelease: true}, func(Settings) {}); got != 1 {
		t.Fatalf("RunCLI(download failure) = %d, want 1", got)
	}
}

func TestRunCLIFailsOnInstall(t *testing.T) {
	oldAPI, oldOS, oldArch, oldProxy, oldExecutable := APIURL, CurrentOS, CurrentArch, netproxy.Global(), Executable
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch, Executable = oldAPI, oldOS, oldArch, oldExecutable
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})
	dir := t.TempDir()
	exe := filepath.Join(dir, "f4")
	if err := os.WriteFile(exe, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	Executable = func() (string, error) { return exe, nil }
	archiveURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/archive" {
			_, _ = w.Write([]byte("not a tar archive"))
			return
		}
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v2.0.0",
			Assets:  []Asset{{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: archiveURL}},
		})
	}))
	defer server.Close()
	APIURL = server.URL
	archiveURL = server.URL + "/archive"

	if got := RunCLI("stable", Settings{Channel: ChannelStable}, Build{Version: "v1.0.0", IsRelease: true}, func(Settings) {}); got != 1 {
		t.Fatalf("RunCLI(install failure) = %d, want 1", got)
	}
}
