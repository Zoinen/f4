package app

import (
	"context"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/vfs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
