package panel

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestSemanticNavigatePathRestoresDeviceURI(t *testing.T) {
	for _, tt := range []struct{ scheme, target string }{
		{"ios", "ios://Alexander’s iPhone/DCIM/a%23b%25%3F%40"},
		{"android", "android://Pixel 3%2FJohn's/sdcard/a%252F"},
	} {
		t.Run(tt.scheme, func(t *testing.T) {
			t.Cleanup(swapFrameManager(t))
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			oldSync := config.App.SyncPanelLoad
			config.App.SyncPanelLoad = false
			t.Cleanup(func() { config.App.SyncPanelLoad = oldSync })
			mounted := &navigationURIVFS{NullVFS: vfs.NewNullVFS(0), uri: tt.target}
			provider := &navigationURIProvider{scheme: tt.scheme, result: mounted}
			if err := vfs.RegisterURIProvider(provider); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { vfs.UnregisterURIProvider(provider.scheme) })
			source := vfs.NewOSVFS(t.TempDir())
			fsp := NewFileSystemPanel(0, 0, 40, 20, source)
			waitForLoad(t, fsp)
			t.Cleanup(func() {
				fsp.cancelProviderOpen()
				if fsp.CancelLoad != nil {
					fsp.CancelLoad()
				}
			})
			pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}
			if !pf.HandleSemanticAction(map[string]any{"action": "panel.navigatePath", "side": 0, "path": tt.target}) {
				t.Fatal("device URI navigation was rejected")
			}
			if fsp.ProviderOpenTask == nil {
				t.Fatal("device URI bypassed the asynchronous provider lifecycle")
			}
			waitForLoad(t, fsp)
			if fsp.Vfs != mounted || fsp.Vfs.GetPath() != tt.target {
				t.Fatalf("mounted path = %q, want %q", fsp.Vfs.GetPath(), tt.target)
			}
			provider.mu.Lock()
			defer provider.mu.Unlock()
			if provider.current != source || provider.target != tt.target {
				t.Fatalf("OpenURI current=%T target=%q", provider.current, provider.target)
			}
		})
	}
}

func TestSemanticNavigatePathPreservesOrdinaryPathSemantics(t *testing.T) {
	for _, target := range []string{"missing-semantic-uri://Phone's name%2FOne/DCIM", "/sdcard", "sdcard"} {
		t.Run(target, func(t *testing.T) {
			t.Cleanup(swapFrameManager(t))
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			source := newQueuedNavigationVFS()
			close(source.releaseFirstRead)
			fsp := NewFileSystemPanel(0, 0, 40, 20, source)
			waitForLoad(t, fsp)
			t.Cleanup(func() {
				if fsp.CancelLoad != nil {
					fsp.CancelLoad()
				}
			})
			pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}
			accepted := pf.HandleSemanticAction(map[string]any{"action": "panel_navigate_path", "side": 0, "path": target})
			wantPath, wantOptimistic := "/sdcard", int64(1)
			if vfs.IsURIPath(target) {
				wantPath, wantOptimistic = "/", 0
				if accepted {
					t.Error("unavailable URI was accepted")
				}
			} else if !accepted {
				t.Error("ordinary path was rejected")
			}
			waitForLoad(t, fsp)
			if got := source.GetPath(); got != wantPath {
				t.Errorf("path = %q, want %q", got, wantPath)
			}
			if got := source.optimisticCalls.Load(); got != wantOptimistic {
				t.Errorf("optimistic calls = %d, want %d", got, wantOptimistic)
			}
			if got := source.checkedPathCalls.Load(); got != 0 {
				t.Errorf("checked path calls = %d, want 0", got)
			}
		})
	}
}
