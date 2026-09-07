package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	gzip "github.com/klauspost/pgzip"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/sevenzip"
	"github.com/unxed/vtui"
	"github.com/unxed/zip"
)

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	UpdatedAt          string `json:"updated_at"`
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	PublishedAt string        `json:"published_at"`
	Body        string        `json:"body"`
	Assets      []githubAsset `json:"assets"`
}

var (
	githubAPIURL = "https://api.github.com/repos/unxed/f4/releases"
	osExecutable = f4Executable
	currentOS    = runtime.GOOS
	currentArch  = runtime.GOARCH
	// Empty except in builds that target a specific C library; see
	// libc_musl.go and updateAssetSuffixes.
	currentLibc = buildLibc

	// zoin-bot: fail a stalled release download instead of leaving the
	// progress screen without an answer after the network disappears.
	updateDownloadIdleTimeout = 30 * time.Second

	// sessionDismissedUpdateKey remembers which update the user
	// declined during the current f4 session, so an interval-driven
	// auto-check does not re-prompt for the same version this run.
	// The dismissal deliberately does NOT persist across restarts —
	// see #374 — and a manual "Check for updates" always ignores it.
	sessionDismissedUpdateKey string
)

type updateReadResult struct {
	n   int
	err error
}

// readUpdateChunk bounds the time spent waiting for the next download chunk.
// Closing the response body in the caller releases a reader that is still
// blocked when the timeout or task cancellation wins the select.
func readUpdateChunk(ctx context.Context, r io.Reader, buf []byte) (int, error) {
	if updateDownloadIdleTimeout <= 0 {
		return r.Read(buf)
	}

	result := make(chan updateReadResult, 1)
	go func() {
		n, err := r.Read(buf)
		result <- updateReadResult{n: n, err: err}
	}()

	timer := time.NewTimer(updateDownloadIdleTimeout)
	defer timer.Stop()
	select {
	case res := <-result:
		return res.n, res.err
	case <-ctx.Done():
		return 0, ctx.Err()
	case <-timer.C:
		return 0, fmt.Errorf("update download stalled for %s", updateDownloadIdleTimeout)
	}
}

func getCurrentVersion() string {
	api := &coreAPI{}
	ver := api.GetVersion()
	parts := strings.Split(ver, "-")
	if len(parts) >= 3 {
		return strings.Join(parts[:len(parts)-1], "-")
	}
	return ver
}

func shouldCheck() bool {
	if AppConfig.UpdateInterval == 0 {
		return false
	}
	if AppConfig.LastUpdateCheck == 0 {
		return true
	}
	last := time.Unix(AppConfig.LastUpdateCheck, 0)
	now := time.Now()
	switch AppConfig.UpdateInterval {
	case 1:
		return true
	case 2:
		return now.Sub(last) >= 24*time.Hour
	case 3:
		return now.Sub(last) >= 7*24*time.Hour
	}
	return false
}

func CheckForUpdates(pf *PanelsFrame, manual bool) {
	if !manual && !shouldCheck() {
		return
	}

	AppConfig.LastUpdateCheck = time.Now().Unix()
	SaveConfig()

	url := githubAPIURL + "/latest"
	if AppConfig.UpdateChannel == 1 {
		url = githubAPIURL + "/tags/nightly"
	}

	vtui.DebugLog("UPDATER: Checking for updates at %s", url)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		reportUpdateError(manual, "Failed to create request: "+err.Error())
		return
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "f4-updater")

	// Everything f4 fetches goes through the configured proxy, if any.
	resp, err := netproxy.HTTPClient(0).Do(req)
	if err != nil {
		reportUpdateError(manual, "Network error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		proxyAuthHeader := resp.Header.Get("Proxy-Authenticate")
		vtui.DebugLog("UPDATER ERROR: HTTP %d from %s (Proxy: %s, Proxy-Authenticate: %q)",
			resp.StatusCode, url, netproxy.Global().Describe(), proxyAuthHeader)
		if resp.StatusCode == http.StatusProxyAuthRequired {
			reportUpdateError(manual, fmt.Sprintf("GitHub API returned status 407 (Proxy Authentication Required).\nProxy: %s, Proxy-Authenticate: %s", netproxy.Global().Describe(), proxyAuthHeader))
			return
		}
		reportUpdateError(manual, fmt.Sprintf("GitHub API returned status %d", resp.StatusCode))
		return
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		reportUpdateError(manual, "Failed to parse API response: "+err.Error())
		return
	}

	assetSuffixes := updateAssetSuffixes(currentOS, currentArch, currentLibc)

	downloadURL, assetUpdated, archiveKind := pickAsset(release.Assets, assetSuffixes)

	if downloadURL == "" {
		reportUpdateError(manual, "No suitable build found for your OS/Arch.")
		return
	}

	needsUpdate := false
	displayVersion := release.TagName
	updateKey := release.TagName

	if AppConfig.UpdateChannel == 1 {
		updateKey = assetUpdated
		displayTime := assetUpdated
		if t, err := time.Parse(time.RFC3339, assetUpdated); err == nil {
			displayTime = t.Local().Format("2006-01-02 15:04")
		} else if len(displayTime) >= 16 {
			displayTime = strings.Replace(displayTime[:16], "T", " ", 1)
		}
		displayVersion = "Nightly (" + displayTime + ")"
		// The asset's updated_at is when the upload finished, which can
		// trail the actual commit by however long the full multi-OS build
		// matrix took — the nightly workflow embeds the commit hash and
		// its own build time in the release body, which is what F1's Help
		// Index shows after installing. Prefer that so the prompt and the
		// post-update version line agree. See #343.
		if commit, builtOn := commitInfoFromReleaseBody(release.Body); commit != "" {
			if builtOn != "" {
				builtOn = formatBuildTimeForDisplay(builtOn)
				displayVersion = "Nightly (" + commit + " [" + builtOn + "])"
			} else {
				displayVersion = "Nightly (" + commit + ")"
			}
		}
		if AppConfig.LastUpdateVersion != updateKey {
			needsUpdate = true
		}
	} else {
		current := getCurrentVersion()
		_, _, buildTimeText := getVCSInfo()
		needsUpdate = stableReleaseNeedsUpdate(release, current, parseUpdateBuildTime(buildTimeText))
	}

	if !needsUpdate {
		if manual {
			vtui.FrameManager.PostTask(func() {
				vtui.ShowMessage(" Auto Update ", "You are using the latest version.", []string{"&Ok"})
			})
		}
		return
	}

	// An update is available, but the user already said "no" to this
	// exact release earlier in this session. Skip the auto-prompt so
	// the next interval-driven check does not nag; a manual "Check
	// for updates" from the settings dialog goes through regardless
	// (see #374).
	if !manual && sessionDismissedUpdateKey == updateKey {
		vtui.DebugLog("UPDATER: skipping prompt — update %q dismissed this session", updateKey)
		return
	}

	vtui.FrameManager.PostTask(func() {
		msg := fmt.Sprintf("An update is available: %s\n\nDo you want to download and install it now?", displayVersion)
		dlg := vtui.ShowMessage(" Auto Update ", msg, []string{"&Yes", "&No"})
		dlg.OnResult = func(code int) {
			if code == 0 {
				performUpdate(pf, downloadURL, archiveKind, release.TagName, updateKey)
				return
			}
			// User declined. Remember only for this session — the
			// next restart (or a manual check) will offer it again.
			// AppConfig.LastUpdateVersion is deliberately NOT touched
			// here: that field is the "we already installed this
			// version" marker and must survive across restarts, while
			// a declined prompt must not (see #374).
			sessionDismissedUpdateKey = updateKey
		}
	})
}

func stableReleaseNeedsUpdate(release githubRelease, current string, buildTime time.Time) bool {
	if release.TagName == current || release.TagName == AppConfig.LastUpdateVersion {
		return false
	}

	// A release tag is an exact version marker. A manually built binary may
	// instead expose only a commit hash, which cannot be compared lexically to
	// a semver release tag. In that case compare the commit/build timestamp to
	// the release publication time, so a newer local checkout is not told to
	// install an older release.
	if !isReleaseVersion(current) && !buildTime.IsZero() {
		published, err := time.Parse(time.RFC3339, release.PublishedAt)
		if err == nil {
			return published.After(buildTime)
		}
	}

	// If metadata is missing or malformed, preserve the safe historical
	// behavior and offer the release rather than silently skipping it.
	return true
}

func parseUpdateBuildTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

// commitInfoFromReleaseBody pulls the commit hash and build time the
// nightly workflow records in the release body:
//
//	**Commit:** `<hash>`
//	**Built on:** `<time>`
//
// Returns empty strings if either line isn't found, so the caller can fall
// back to the asset timestamp.
func commitInfoFromReleaseBody(body string) (commit, builtOn string) {
	commit = extractBacktickedField(body, "**Commit:**")
	builtOn = extractBacktickedField(body, "**Built on:**")
	return commit, builtOn
}

func extractBacktickedField(body, label string) string {
	i := strings.Index(body, label)
	if i == -1 {
		return ""
	}
	rest := body[i+len(label):]
	start := strings.Index(rest, "`")
	if start == -1 {
		return ""
	}
	rest = rest[start+1:]
	end := strings.Index(rest, "`")
	if end == -1 {
		return ""
	}
	return rest[:end]
}

func reportUpdateError(manual bool, msg string) {
	vtui.DebugLog("UPDATER ERROR: %s", msg)
	if manual {
		vtui.FrameManager.PostTask(func() {
			vtui.ShowMessage(" Update Error ", msg, []string{"&Ok"})
		})
	}
}

func performUpdate(pf *PanelsFrame, url, archiveKind, newTag, publishedAt string) {
	if pf == nil {
		return
	}
	pf.RunProgressTask(" Updating f4 ", "Downloading...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		exePath, err := osExecutable()
		if err != nil {
			return fmt.Errorf("failed to get executable path: %w", err)
		}
		exePath, err = filepath.EvalSymlinks(exePath)
		if err != nil {
			return fmt.Errorf("failed to resolve symlinks for executable: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "f4-updater")

		resp, err := netproxy.HTTPClient(0).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			proxyAuthHeader := resp.Header.Get("Proxy-Authenticate")
			vtui.DebugLog("UPDATER ERROR: Download failed HTTP %d from %s (Proxy: %s, Proxy-Authenticate: %q)",
				resp.StatusCode, url, netproxy.Global().Describe(), proxyAuthHeader)
			if resp.StatusCode == http.StatusProxyAuthRequired {
				return fmt.Errorf("download failed with status 407 (Proxy Authentication Required).\nProxy: %s, Proxy-Authenticate: %s", netproxy.Global().Describe(), proxyAuthHeader)
			}
			return fmt.Errorf("download failed with status %d", resp.StatusCode)
		}

		contentLength := resp.ContentLength
		var archiveData bytes.Buffer
		buf := make([]byte, 32*1024)
		var downloaded int64

		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			n, readErr := readUpdateChunk(ctx, resp.Body, buf)
			if n > 0 {
				archiveData.Write(buf[:n])
				downloaded += int64(n)
				pct := 0
				if contentLength > 0 {
					pct = int((downloaded * 100) / contentLength)
				}
				update("Downloading update...", pct)
			}
			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				return readErr
			}
		}

		update("Extracting and installing...", -1)

		exeDir := filepath.Dir(exePath)
		if updateDirNeedsElevation(exeDir) {
			vtui.DebugLog("UPDATER: %q is not writable, requesting UAC elevation", exeDir)
			err = runElevatedUpdate(archiveData.Bytes(), archiveKind)
		} else {
			err = extractUpdateArchive(archiveData.Bytes(), archiveKind, exeDir)
			if err != nil && isPermissionErrorForUpdate(err) {
				vtui.DebugLog("UPDATER: extraction needs elevation, retrying through UAC: %v", err)
				err = runElevatedUpdate(archiveData.Bytes(), archiveKind)
			}
		}

		if err != nil {
			return fmt.Errorf("failed to extract/install update: %w\n(Close other f4 instances, check Task Manager for ghost f4 processes, or try running as admin/root)", err)
		}

		return nil
	}, func(err error) {
		if err != nil {
			if err != context.Canceled {
				vtui.ShowMessage(" Update Failed ", err.Error(), []string{"&Ok"})
			}
			return
		}

		if AppConfig.UpdateChannel == 1 {
			AppConfig.LastUpdateVersion = publishedAt
		} else {
			AppConfig.LastUpdateVersion = newTag
		}
		SaveConfig()

		dlg := vtui.ShowMessage(" Update Successful ", "f4 has been updated successfully.\nPlease restart the application to apply changes.", []string{"E&xit now", "&Later"})
		dlg.OnResult = func(code int) {
			if code == 0 {
				cancelOperationsForShutdown()
				vtui.FrameManager.Shutdown()
			}
		}
	})
}

func extractUpdateArchive(data []byte, archiveKind, destDir string) error {
	switch archiveKind {
	case "7z":
		return extract7zToDir(data, destDir)
	case "targz":
		return extractTarGzToDir(data, destDir)
	default:
		// 4 workers: benchmark-optimal.
		return extractZipToDirParallel(data, destDir, min(runtime.GOMAXPROCS(0), 4))
	}
}

func writeFileSafe(targetPath string, r io.Reader, mode os.FileMode) error {
	primaryOldPath := targetPath + ".old"
	oldPath := primaryOldPath

	err := os.Remove(oldPath)
	if err != nil && os.IsPermission(err) && vfs.GetSudoClient().IsAvailable() {
		_ = vfs.GetSudoClient().Remove(oldPath)
	}

	if _, err := os.Stat(oldPath); err == nil {
		for i := 1; i < 1000; i++ {
			cand := fmt.Sprintf("%s.%d", primaryOldPath, i)
			err := os.Remove(cand)
			if err != nil && os.IsPermission(err) && vfs.GetSudoClient().IsAvailable() {
				_ = vfs.GetSudoClient().Remove(cand)
			}
			if _, err := os.Stat(cand); os.IsNotExist(err) {
				oldPath = cand
				break
			}
		}
	}

	if _, err := os.Stat(targetPath); err == nil {
		errRename := os.Rename(targetPath, oldPath)
		if errRename != nil && os.IsPermission(errRename) && vfs.GetSudoClient().IsAvailable() {
			_ = vfs.GetSudoClient().Rename(targetPath, oldPath)
		}
	}

	dir := filepath.Dir(targetPath)
	// #nosec G301 -- updater targets may be shared/system installations whose directories must remain searchable by other users.
	errMkdir := os.MkdirAll(dir, 0755)
	if errMkdir != nil && os.IsPermission(errMkdir) && vfs.GetSudoClient().IsAvailable() {
		_ = vfs.GetSudoClient().MkDir(dir, 0755)
	}

	var f *os.File
	f, err = os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil && os.IsPermission(err) && vfs.GetSudoClient().IsAvailable() {
		vtui.DebugLog("UPDATER: Permission denied for %q, attempting elevated write via sudo...", targetPath)
		f, err = vfs.GetSudoClient().Open(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, uint32(mode))
	}

	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, r)
	if err != nil {
		return err
	}

	errRemove := os.Remove(primaryOldPath)
	if errRemove != nil && os.IsPermission(errRemove) && vfs.GetSudoClient().IsAvailable() {
		_ = vfs.GetSudoClient().Remove(primaryOldPath)
	}

	return nil
}

func sanitizeExtractPath(name, destDir string) (string, error) {
	// Archive paths are always slash-separated
	if strings.ContainsRune(name, '\\') || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("invalid path in archive: %s", name)
	}
	cleanName := path.Clean(name)
	if path.IsAbs(cleanName) || strings.HasPrefix(cleanName, "../") || cleanName == ".." {
		return "", fmt.Errorf("invalid path in archive: %s", name)
	}
	return filepath.Join(destDir, filepath.FromSlash(cleanName)), nil
}

type archiveEntry struct {
	name  string
	isDir bool
	mode  os.FileMode
	open  func() (io.ReadCloser, error)
}

func extractEntry(e archiveEntry, destDir string) error {
	targetPath, err := sanitizeExtractPath(e.name, destDir)
	if err != nil {
		return nil // Skip malicious/invalid paths
	}

	if e.isDir {
		// #nosec G301 -- release archive directories may belong to a shared installation and must remain searchable by other users.
		errMkdir := os.MkdirAll(targetPath, 0755)
		if errMkdir != nil && os.IsPermission(errMkdir) && vfs.GetSudoClient().IsAvailable() {
			_ = vfs.GetSudoClient().MkDir(targetPath, 0755)
		}
		return nil
	}

	rc, err := e.open()
	if err != nil {
		return err
	}

	mode := e.mode
	if mode == 0 {
		mode = 0644
	}
	err = writeFileSafe(targetPath, rc, mode)
	rc.Close()
	return err
}

func extractTarGzToDir(data []byte, destDir string) error {
	r := bytes.NewReader(data)
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		// tar.Header.Mode is a signed container, but only its low permission
		// bits are meaningful to the extracted file.
		// #nosec G115 -- masking to 0777 bounds the os.FileMode conversion.
		mode := os.FileMode(hdr.Mode & 0o777)
		if err := extractEntry(archiveEntry{
			name:  hdr.Name,
			isDir: hdr.Typeflag == tar.TypeDir,
			mode:  mode,
			open:  func() (io.ReadCloser, error) { return io.NopCloser(tr), nil },
		}, destDir); err != nil {
			return err
		}
	}
}

func extractZipToDir(data []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		if err := extractEntry(archiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}, destDir); err != nil {
			return err
		}
	}
	return nil
}

// extractZipToDirParallel: extractZipToDir with entries in parallel; falls back when <2 entries.
func extractZipToDirParallel(data []byte, destDir string, workers int) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if workers < 2 || len(zr.File) < 2 {
		return extractZipToDir(data, destDir)
	}
	if workers > len(zr.File) {
		workers = len(zr.File)
	}

	entries := make([]archiveEntry, len(zr.File))
	for i, f := range zr.File {
		entries[i] = archiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}
	}

	errCh := make(chan error, 1)
	done := make(chan struct{})
	jobs := make(chan archiveEntry)
	go func() {
		defer close(jobs)
		for _, e := range entries {
			select {
			case jobs <- e:
			case <-done:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for e := range jobs {
				if err := extractEntry(e, destDir); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
			}
		}()
	}
	wg.Wait()
	close(done)
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// pickAsset returns the first release asset whose name ends with one of the
// suffixes, trying suffixes in order. Returns an empty url when nothing matches.
// updateAssetSuffixes returns the release asset suffixes to look for, most
// preferred first.
//
// Linux is published in two flavors that share a GOOS and a GOARCH: the
// generic artifacts are fully static (they start anywhere, including on musl
// systems, but carry no FFI, so no GPU, Wayland or Ebiten backend), while the
// musl artifacts link against musl's libc and keep FFI. Only the running
// binary knows which one it is, hence libc.
//
// A musl build therefore asks for the musl asset first and falls back to the
// generic one, which is a real downgrade -- FFI disappears -- but a working
// f4 rather than a failed update. The fallback is safe in that direction only:
// the generic artifact is static, so it runs on Alpine. The reverse is not
// true, and does not happen, because a musl asset name ends in
// "-musl-<arch>.tar.gz" and so never matches a glibc build's suffix.
func updateAssetSuffixes(goos, goarch, libc string) []string {
	if goos == "windows" {
		// Windows: priority order .7z, then .zip.
		return []string{
			fmt.Sprintf("-%s-%s.7z", goos, goarch),
			fmt.Sprintf("-%s-%s.zip", goos, goarch),
		}
	}

	// Android is published as the Termux build and named after it.
	if goos == "android" {
		goos = "termux"
	}
	generic := fmt.Sprintf("-%s-%s.tar.gz", goos, goarch)
	if goos == "linux" && libc != "" {
		return []string{
			fmt.Sprintf("-%s-%s-%s.tar.gz", goos, libc, goarch),
			generic,
		}
	}
	return []string{generic}
}

func pickAsset(assets []githubAsset, suffixes []string) (url, updatedAt, kind string) {
	for _, suffix := range suffixes {
		for _, a := range assets {
			if strings.HasSuffix(a.Name, suffix) {
				return a.BrowserDownloadURL, a.UpdatedAt, archiveKindForSuffix(suffix)
			}
		}
	}
	return "", "", ""
}

func archiveKindForSuffix(suffix string) string {
	switch {
	case strings.HasSuffix(suffix, ".7z"):
		return "7z"
	case strings.HasSuffix(suffix, ".tar.gz"):
		return "targz"
	default:
		return "zip"
	}
}

func extract7zToDir(data []byte, destDir string) error {
	szr, err := sevenzip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range szr.File {
		if err := extractEntry(archiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}, destDir); err != nil {
			return err
		}
	}
	return nil
}
