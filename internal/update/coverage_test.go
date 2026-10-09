package update

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/netproxy"
)

func TestUpdateHelpersCoverChannelsAndAssets(t *testing.T) {
	if got := ChannelName(ChannelStable); got != "stable" {
		t.Fatalf("ChannelName(stable) = %q, want stable", got)
	}
	if got := ChannelName(ChannelNightly); got != "nightly" {
		t.Fatalf("ChannelName(nightly) = %q, want nightly", got)
	}
	if got := ChannelName(99); got != "stable" {
		t.Fatalf("ChannelName(99) = %q, want stable", got)
	}

	channelTests := []struct {
		arg        string
		configured int
		want       int
		explicit   bool
		wantErr    bool
	}{
		{arg: "", configured: ChannelNightly, want: ChannelNightly},
		{arg: " latest ", configured: ChannelNightly, want: ChannelStable, explicit: true},
		{arg: "NIGHTLY", configured: ChannelStable, want: ChannelNightly, explicit: true},
		{arg: "beta", configured: ChannelStable, wantErr: true},
	}
	for _, tt := range channelTests {
		t.Run(tt.arg, func(t *testing.T) {
			got, explicit, err := ParseChannelArg(tt.arg, tt.configured)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseChannelArg(%q) error = %v, want error %v", tt.arg, err, tt.wantErr)
			}
			if err == nil && (got != tt.want || explicit != tt.explicit) {
				t.Fatalf("ParseChannelArg(%q) = %d, %v; want %d, %v", tt.arg, got, explicit, tt.want, tt.explicit)
			}
		})
	}

	if got := assetSuffixes("android", "arm64", ""); !reflect.DeepEqual(got, []string{"-termux-arm64.tar.gz"}) {
		t.Fatalf("Android asset suffixes = %v", got)
	}
	// The armv7 Termux asset is published as f4-termux-arm.* (build.yml
	// build-termux names it after GOARCH); the updater must ask for that.
	if got := assetSuffixes("android", "arm", ""); !reflect.DeepEqual(got, []string{"-termux-arm.tar.gz"}) {
		t.Fatalf("Android arm asset suffixes = %v", got)
	}
	if got := assetSuffixes("linux", "amd64", "musl"); !reflect.DeepEqual(got, []string{
		"-linux-musl-amd64.tar.gz",
		"-linux-amd64.tar.gz",
	}) {
		t.Fatalf("musl asset suffixes = %v", got)
	}

	assets := []Asset{
		{Name: "f4-linux-amd64.7z", BrowserDownloadURL: "7z"},
		{Name: "f4-linux-amd64.zip", BrowserDownloadURL: "zip"},
	}
	url, _, kind := pickAsset(assets, []string{"-amd64.zip", "-amd64.7z"})
	if url != "zip" || kind != "zip" {
		t.Fatalf("pickAsset() = %q, %q; want zip, zip", url, kind)
	}
	if url, updated, kind := pickAsset(assets, []string{"-missing"}); url != "" || updated != "" || kind != "" {
		t.Fatalf("pickAsset(missing) = %q, %q, %q; want empty result", url, updated, kind)
	}
	for suffix, want := range map[string]string{"build.7z": "7z", "build.tar.gz": "targz", "build.zip": "zip"} {
		if got := archiveKindForSuffix(suffix); got != want {
			t.Errorf("archiveKindForSuffix(%q) = %q, want %q", suffix, got, want)
		}
	}
}

func TestUpdateNightlyDisplayAndReleaseBodyParsing(t *testing.T) {
	builtOn := "2026-08-23T06:49:17Z"
	wantBuilt := FormatBuildTime(builtOn)
	tests := []struct {
		name         string
		release      Release
		assetUpdated string
		want         string
	}{
		{
			name:    "commit and build time",
			release: Release{Body: "**Commit:** `abc123`\n**Built on:** `" + builtOn + "`"},
			want:    "Nightly (abc123 [" + wantBuilt + "])"},
		{
			name:    "commit without build time",
			release: Release{Body: "**Commit:** `abc123`"},
			want:    "Nightly (abc123)"},
		{
			name:         "asset timestamp",
			release:      Release{},
			assetUpdated: "2026-08-23T06:49:17Z",
			want:         "Nightly (" + FormatBuildTime("2026-08-23T06:49:17Z") + ")"},
		{
			name:         "legacy timestamp",
			release:      Release{},
			assetUpdated: "2026-08-23T06:49:17",
			want:         "Nightly (2026-08-23 06:49)"},
		{
			name:         "unknown timestamp",
			release:      Release{},
			assetUpdated: "unknown",
			want:         "Nightly (unknown)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nightlyDisplayVersion(tt.release, tt.assetUpdated); got != tt.want {
				t.Fatalf("nightlyDisplayVersion() = %q, want %q", got, tt.want)
			}
		})
	}

	if commit, built := commitInfoFromReleaseBody("**Built on:** `time`"); commit != "" || built != "time" {
		t.Fatalf("commitInfoFromReleaseBody() = %q, %q", commit, built)
	}
	if got := extractBacktickedField("label without a closing mark", "label"); got != "" {
		t.Fatalf("extractBacktickedField() = %q, want empty", got)
	}
}

func TestUpdateRateLimitMessage(t *testing.T) {
	reset := int64(1790000000)
	withReset := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     make(http.Header),
	}
	withReset.Header.Set("X-RateLimit-Remaining", "0")
	withReset.Header.Set("X-RateLimit-Reset", "1790000000")
	msg := rateLimitMessage(withReset)
	if !strings.Contains(msg, "used them all up") || !strings.Contains(msg, time.Unix(reset, 0).Local().Format("15:04")) {
		t.Fatalf("rateLimitMessage() = %q", msg)
	}

	withoutReset := &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header)}
	withoutReset.Header.Set("X-RateLimit-Remaining", "0")
	if msg := rateLimitMessage(withoutReset); !strings.Contains(msg, "used them all up") || strings.Contains(msg, "Try again") {
		t.Fatalf("rateLimitMessage() without reset = %q", msg)
	}
	for _, resp := range []*http.Response{
		{StatusCode: http.StatusForbidden, Header: make(http.Header)},
		{StatusCode: http.StatusOK, Header: make(http.Header)},
	} {
		resp.Header.Set("X-RateLimit-Remaining", map[int]string{http.StatusForbidden: "1", http.StatusOK: "0"}[resp.StatusCode])
		if msg := rateLimitMessage(resp); msg != "" {
			t.Errorf("rateLimitMessage(%d) = %q, want empty", resp.StatusCode, msg)
		}
	}
}

type blockedUpdateReader struct {
	release chan struct{}
}

func (r *blockedUpdateReader) Read([]byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

func TestUpdateReadChunkCancellationAndDirectRead(t *testing.T) {
	oldTimeout := DownloadIdleTimeout
	t.Cleanup(func() { DownloadIdleTimeout = oldTimeout })

	DownloadIdleTimeout = 0
	buf := make([]byte, 4)
	n, err := readChunk(context.Background(), strings.NewReader("data"), buf)
	if n != 4 || err != nil || string(buf) != "data" {
		t.Fatalf("readChunk() = %d, %v, %q", n, err, buf)
	}

	DownloadIdleTimeout = time.Minute
	reader := &blockedUpdateReader{release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = readChunk(ctx, reader, buf)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readChunk(canceled) error = %v, want context canceled", err)
	}
	close(reader.release)
}

func TestUpdateCheckAndDownloadUseSelectedRelease(t *testing.T) {
	oldAPIURL, oldOS, oldArch, oldProxy := APIURL, CurrentOS, CurrentArch, netproxy.Global()
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch = oldAPIURL, oldOS, oldArch
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/latest":
			_, _ = io.WriteString(w, `{"tag_name":"v2.0.0","published_at":"2026-08-22T00:00:00Z","assets":[{"name":"f4-linux-amd64.tar.gz","browser_download_url":"https://example/stable"}]}`)
		case "/tags/nightly":
			_, _ = io.WriteString(w, `{"tag_name":"nightly","body":"**Commit:** `+"`nightly-sha`"+`\n**Built on:** `+"`2026-08-23T06:49:17Z`"+`","assets":[{"name":"f4-linux-amd64.tar.gz","browser_download_url":"https://example/archive","updated_at":"2026-08-24T00:00:00Z"}]}`)
		case "/archive":
			w.Header().Set("Content-Length", "4")
			_, _ = io.WriteString(w, "data")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	APIURL = server.URL

	stable, err := Check(context.Background(), Settings{Channel: ChannelStable}, Build{Version: "v1.0.0", IsRelease: true})
	if err != nil {
		t.Fatalf("stable Check() error: %v", err)
	}
	if stable.DownloadURL == "" || stable.ArchiveKind != "targz" || !stable.NeedsUpdate {
		t.Fatalf("stable Check() = %+v", stable)
	}

	nightly, err := Check(context.Background(), Settings{Channel: ChannelNightly}, Build{})
	if err != nil {
		t.Fatalf("nightly Check() error: %v", err)
	}
	if nightly.UpdateKey != "2026-08-24T00:00:00Z" || !strings.HasPrefix(nightly.DisplayVersion, "Nightly (nightly-sha [") {
		t.Fatalf("nightly Check() = %+v", nightly)
	}

	var progress []int
	data, err := Download(context.Background(), server.URL+"/archive", func(percent int) { progress = append(progress, percent) })
	if err != nil || string(data) != "data" {
		t.Fatalf("Download() = %q, %v", data, err)
	}
	if len(progress) == 0 {
		t.Fatal("Download() did not report progress")
	}
}

func TestUpdateTargetDirResolvesExecutable(t *testing.T) {
	realDir := t.TempDir()
	realExe := filepath.Join(realDir, "f4")
	if err := os.WriteFile(realExe, []byte("binary"), 0755); err != nil { // #nosec G306 -- test fixture represents an executable.
		t.Fatal(err)
	}
	linkDir := t.TempDir()
	linkExe := filepath.Join(linkDir, "f4")
	if err := os.Symlink(realExe, linkExe); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	resolvedExe, err := filepath.EvalSymlinks(linkExe)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Dir(resolvedExe)

	oldExecutable := Executable
	Executable = func() (string, error) { return linkExe, nil }
	t.Cleanup(func() { Executable = oldExecutable })

	got, err := TargetDir()
	if err != nil || got != wantDir {
		t.Fatalf("TargetDir() = %q, %v; want %q", got, err, wantDir)
	}
}

func TestUpdateExtractDispatchesArchiveKinds(t *testing.T) {
	for _, kind := range []string{"7z", "targz", "zip", "unknown"} {
		if err := extract([]byte("not an archive"), kind, t.TempDir()); err == nil {
			t.Errorf("extract(%q) accepted invalid data", kind)
		}
	}
}
