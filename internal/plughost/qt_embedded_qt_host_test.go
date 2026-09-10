package plughost

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func gzipQtHostFixture(t *testing.T, content string) []byte {
	t.Helper()
	var payload bytes.Buffer
	writer, err := gzip.NewWriterLevel(&payload, gzip.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return payload.Bytes()
}

func TestMaterializeEmbeddedQtHostExtractsAndReuses(t *testing.T) {
	payload := gzipQtHostFixture(t, "portable qt host")
	cacheRoot := t.TempDir()
	path, err := materializeEmbeddedQtHost(payload, cacheRoot, "linux")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "portable qt host" {
		t.Fatalf("extracted content = %q", content)
	}
	if filepath.Ext(path) != "" {
		t.Fatalf("Linux host path unexpectedly has an extension: %s", path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	pathAgain, err := materializeEmbeddedQtHost(payload, cacheRoot, "linux")
	if err != nil {
		t.Fatal(err)
	}
	infoAgain, err := os.Stat(pathAgain)
	if err != nil {
		t.Fatal(err)
	}
	if pathAgain != path || !os.SameFile(info, infoAgain) {
		t.Fatalf("cached host was not reused: %q then %q", path, pathAgain)
	}

	updatedPath, err := materializeEmbeddedQtHost(
		gzipQtHostFixture(t, "updated qt host"), cacheRoot, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if updatedPath == path {
		t.Fatal("a changed payload reused the stale content-addressed path")
	}
}

func TestMaterializeEmbeddedQtHostConcurrentFirstLaunch(t *testing.T) {
	payload := gzipQtHostFixture(t, "concurrent portable qt host")
	cacheRoot := t.TempDir()
	const workers = 8
	paths := make(chan string, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			path, err := materializeEmbeddedQtHost(payload, cacheRoot, "windows")
			paths <- path
			errs <- err
		}()
	}
	group.Wait()
	close(paths)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var want string
	for path := range paths {
		if filepath.Ext(path) != ".exe" {
			t.Fatalf("Windows host path = %q", path)
		}
		if want == "" {
			want = path
		} else if path != want {
			t.Fatalf("concurrent extraction returned %q and %q", want, path)
		}
	}
	content, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "concurrent portable qt host" {
		t.Fatalf("concurrent extracted content = %q", content)
	}
}

func TestMaterializeEmbeddedQtHostRejectsCorruptPayload(t *testing.T) {
	if _, err := materializeEmbeddedQtHost([]byte("not gzip"), t.TempDir(), "linux"); err == nil {
		t.Fatal("corrupt embedded payload was accepted")
	}
}

func TestFindExtUiPathPrefersEmbeddedHostOverDevelopmentBuild(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("macOS uses a signed application bundle")
	}
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("F4_EXT_UI_PATH", "")
	t.Setenv(embeddedQtHostCacheEnv, filepath.Join(root, "cache"))
	name := "f4-qt-host"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	stale := filepath.Join(root, "qt", "host", "build", "bin", "Release", name)
	if err := os.MkdirAll(filepath.Dir(stale), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("stale development host"), 0700); err != nil {
		t.Fatal(err)
	}
	original := embeddedQtHostGzip
	t.Cleanup(func() { embeddedQtHostGzip = original })
	embeddedQtHostGzip = gzipQtHostFixture(t, "matching embedded host")
	got, err := findExtUiPath("qt")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "matching embedded host" {
		t.Fatalf("selected %q (%q) instead of the embedded host", got, content)
	}
	embeddedQtHostGzip = nil
	got, err = findExtUiPath("qt")
	if err != nil || got != stale {
		t.Fatalf("non-portable development fallback = %q, %v; want %q", got, err, stale)
	}
}
