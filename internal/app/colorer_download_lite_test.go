//go:build lite

package app

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestColorerLiteDownloadReportsUnavailableWithoutNetwork(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	oldURL := editor.ColorerDownloadURL
	editor.ColorerDownloadURL = server.URL
	t.Cleanup(func() { editor.ColorerDownloadURL = oldURL })

	var completions int
	editor.DownloadColorerSchemas(pf, func(success bool) {
		completions++
		if success {
			t.Error("lite schema download must report unavailable")
		}
	})
	if completions != 1 {
		t.Fatalf("completion callbacks = %d, want one immediate failure", completions)
	}
	editor.DownloadColorerSchemas(pf, nil)
	if got := requests.Load(); got != 0 {
		t.Fatalf("lite schema download made %d HTTP requests, want none", got)
	}
	if editor.SchemasExist() {
		t.Fatal("lite schema download must not enable Colorer")
	}
	t.Log("[FIX:lite-app] schema downloader fails immediately without fetching")
}
