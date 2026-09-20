package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/vfs"
)

func TestAIDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("patcher"))
		case "/bad":
			w.WriteHeader(http.StatusTeapot)
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", vtvibeAPMaxDownload+1)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	got, err := aiDownload(context.Background(), server.URL+"/ok")
	if err != nil || string(got) != "patcher" {
		t.Fatalf("successful download = %q, %v; want patcher, nil", got, err)
	}

	if _, err := aiDownload(context.Background(), server.URL+"/bad"); err == nil {
		t.Fatal("non-200 response returned nil error")
	}

	got, err = aiDownload(context.Background(), server.URL+"/large")
	if err != nil || len(got) != vtvibeAPMaxDownload {
		t.Fatalf("limited download length = %d, %v; want %d, nil", len(got), err, vtvibeAPMaxDownload)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := aiDownload(canceled, server.URL+"/ok"); err == nil {
		t.Fatal("canceled download returned nil error")
	}
	if _, err := aiDownload(context.Background(), "://invalid-url"); err == nil {
		t.Fatal("invalid URL returned nil error")
	}
}

func TestAIPatchTargetDir(t *testing.T) {
	if root, ok := aiPatchTargetDir(nil); ok || root != "" {
		t.Fatalf("nil panels frame = %q, %v; want empty, false", root, ok)
	}
	if root, ok := aiPatchTargetDir(&panel.PanelsFrame{}); ok || root != "" {
		t.Fatalf("empty panels frame = %q, %v; want empty, false", root, ok)
	}

	pf := &panel.PanelsFrame{}
	pf.Panels[0] = &panel.FileSystemPanel{Vfs: &aiVFSWrapper{AIVFS: vtvibe.NewVFS(vtvibe.NewSession())}}
	pf.Panels[1] = &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir())}
	root, ok := aiPatchTargetDir(pf)
	if !ok || root == "" {
		t.Fatalf("OS panel target = %q, %v; want a local directory", root, ok)
	}
}

func TestAILastAnswerPath(t *testing.T) {
	if got := aiLastAnswerPath(&vtvibe.Session{}); got != "" {
		t.Fatalf("empty session path = %q; want empty", got)
	}
	if got := aiLastAnswerPath(vtvibe.NewSession()); got != "/chat/0001-model.md" {
		t.Fatalf("new session path = %q; want /chat/0001-model.md", got)
	}
}

func writeVtvibeINI(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(config.GetF4ConfigDir(), vtvibeIniName)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write vtvibe.ini: %v", err)
	}
	return path
}

func TestAIEnsurePatcher_ConfigCacheAndDownload(t *testing.T) {
	setupPortableIni(t, "0")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid":
			_, _ = w.Write([]byte("def apply_patch():\n    pass\n"))
		case "/bad":
			_, _ = w.Write([]byte("not an ap patcher"))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	custom := filepath.Join(t.TempDir(), "custom-ap.py")
	if err := os.WriteFile(custom, []byte("custom"), 0600); err != nil {
		t.Fatalf("write custom patcher: %v", err)
	}
	writeVtvibeINI(t, "[general]\nap_patcher = "+custom+"\n")
	updates := 0
	got, err := aiEnsurePatcher(context.Background(), func(string, int) { updates++ })
	if err != nil || got != custom {
		t.Fatalf("configured patcher = %q, %v; want %q, nil", got, err, custom)
	}
	if updates != 0 {
		t.Fatalf("configured patcher reported %d download updates; want none", updates)
	}

	if err := os.Remove(custom); err != nil {
		t.Fatalf("remove custom patcher: %v", err)
	}
	if _, err := aiEnsurePatcher(context.Background(), func(string, int) {}); err == nil {
		t.Fatal("missing configured patcher returned nil error")
	}

	cachePath := aiPatcherPath()
	writeVtvibeINI(t, "[general]\nap_url = "+server.URL+"/valid\n")
	updates = 0
	got, err = aiEnsurePatcher(context.Background(), func(string, int) { updates++ })
	if err != nil || got != cachePath {
		t.Fatalf("downloaded patcher = %q, %v; want %q, nil", got, err, cachePath)
	}
	if updates != 1 {
		t.Fatalf("download reported %d updates; want one", updates)
	}
	data, err := os.ReadFile(cachePath)
	if err != nil || !strings.Contains(string(data), "def apply_patch(") {
		t.Fatalf("cached patcher = %q, %v; want recognizable script", data, err)
	}

	updates = 0
	if got, err := aiEnsurePatcher(context.Background(), func(string, int) { updates++ }); err != nil || got != cachePath {
		t.Fatalf("cache hit = %q, %v; want %q, nil", got, err, cachePath)
	}
	if updates != 0 {
		t.Fatalf("cache hit reported %d download updates; want none", updates)
	}

	if err := os.Remove(cachePath); err != nil {
		t.Fatalf("remove cached patcher: %v", err)
	}
	writeVtvibeINI(t, "[general]\nap_url = "+server.URL+"/bad\n")
	if _, err := aiEnsurePatcher(context.Background(), func(string, int) {}); err == nil {
		t.Fatal("unrecognizable download returned nil error")
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("bad download left cache at %q, stat error %v", cachePath, err)
	}
}

func TestAIPythonPath_HonorsConfiguredInterpreter(t *testing.T) {
	setupPortableIni(t, "0")
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	writeVtvibeINI(t, "[general]\npython = "+executable+"\n")

	got, err := aiPythonPath()
	if err != nil || filepath.Clean(got) != filepath.Clean(executable) {
		t.Fatalf("configured Python = %q, %v; want %q, nil", got, err, executable)
	}
}

func TestAIWriteContextFile_WritesThroughSessionVFS(t *testing.T) {
	name := "coverage-vtvibe-ap.md"
	want := []byte("patch specification")
	if err := aiWriteContextFile(name, want); err != nil {
		t.Fatalf("aiWriteContextFile: %v", err)
	}

	r, err := vtvibe.NewVFS(aiSession()).Open(context.Background(), "/ctx/"+name)
	if err != nil {
		t.Fatalf("open attached context: %v", err)
	}
	defer func() { _ = r.Close() }()
	got := make([]byte, len(want))
	if n, err := r.ReadAt(context.Background(), got, 0); err != nil || n != len(want) || string(got) != string(want) {
		t.Fatalf("attached context = %q, n=%d, err=%v; want %q", got, n, err, want)
	}
}
