package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseUpdateChannelArg(t *testing.T) {
	cases := []struct {
		arg      string
		want     int
		explicit bool
		wantErr  bool
	}{
		{"", updateChannelNightly, false, false}, // empty means the configured channel
		{"stable", updateChannelStable, true, false},
		{"latest", updateChannelStable, true, false},
		{"Nightly", updateChannelNightly, true, false},
		{" nightly ", updateChannelNightly, true, false},
		{"beta", 0, false, true},
	}
	for _, c := range cases {
		got, explicit, err := parseUpdateChannelArg(c.arg, updateChannelNightly)
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
	oldCfg := AppConfig
	origOS, origArch, origAPI := currentOS, currentArch, githubAPIURL
	t.Cleanup(func() {
		AppConfig = oldCfg
		currentOS, currentArch, githubAPIURL = origOS, origArch, origAPI
	})
	currentOS, currentArch = "linux", "amd64"
	AppConfig.LastUpdateVersion = ""

	var asked string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Path
		release := githubRelease{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Body:        "**Commit:** `abc1234`\n**Built on:** `2030-01-01 00:00`",
			Assets: []githubAsset{{
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
	githubAPIURL = ts.URL + "/repos/unxed/f4/releases"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cand, err := fetchUpdateCandidate(ctx, updateChannelNightly)
	if err != nil {
		t.Fatalf("nightly: %v", err)
	}
	if asked != "/repos/unxed/f4/releases/tags/nightly" {
		t.Errorf("nightly asked %q", asked)
	}
	// The build time renders in local time, so match on the commit instead.
	if !strings.HasPrefix(cand.displayVersion, "Nightly (abc1234 [") {
		t.Errorf("nightly display version %q", cand.displayVersion)
	}
	if cand.updateKey != "2030-01-01T00:00:00Z" || !cand.needsUpdate {
		t.Errorf("nightly key %q needsUpdate %v", cand.updateKey, cand.needsUpdate)
	}
	if cand.archiveKind != "targz" || cand.downloadURL != "http://mock/f4.tar.gz" {
		t.Errorf("nightly asset %q %q", cand.archiveKind, cand.downloadURL)
	}

	cand, err = fetchUpdateCandidate(ctx, updateChannelStable)
	if err != nil {
		t.Fatalf("stable: %v", err)
	}
	if asked != "/repos/unxed/f4/releases/latest" {
		t.Errorf("stable asked %q", asked)
	}
	if cand.displayVersion != "v100.0.0" || cand.updateKey != "v100.0.0" {
		t.Errorf("stable version %q key %q", cand.displayVersion, cand.updateKey)
	}
}

// A 403 from the spent request limit has to explain itself rather than show a
// number: it is what anyone who checks for updates often runs into.
func TestFetchUpdateCandidateExplainsRateLimit(t *testing.T) {
	origAPI := githubAPIURL
	t.Cleanup(func() { githubAPIURL = origAPI })

	reset := time.Now().Add(42 * time.Minute)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	githubAPIURL = ts.URL

	_, err := fetchUpdateCandidate(context.Background(), updateChannelStable)
	if err == nil {
		t.Fatal("rate-limited check must fail")
	}
	if !strings.Contains(err.Error(), "60 per hour") || !strings.Contains(err.Error(), reset.Format("15:04")) {
		t.Errorf("unhelpful rate limit message: %q", err)
	}
}
