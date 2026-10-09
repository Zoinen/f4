package panel

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	androidfs "github.com/unxed/f4/plugins/android"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type delayedAndroidDevices struct {
	block   atomic.Bool
	started chan struct{}
	release chan struct{}
}

func (s *delayedAndroidDevices) ListDevices(ctx context.Context) ([]androidfs.DeviceInfo, error) {
	serial := "cached-phone"
	if s.block.Load() {
		close(s.started)
		select {
		case <-s.release:
			serial = "fresh-phone"
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return []androidfs.DeviceInfo{{Serial: serial, State: androidfs.DeviceStateOnline}}, nil
}

func TestAndroidParentNavigationShowsCachedDevicesDuringDiscovery(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldSync := config.App.SyncPanelLoad
	config.App.SyncPanelLoad = false
	t.Cleanup(func() { config.App.SyncPanelLoad = oldSync })
	source := &delayedAndroidDevices{started: make(chan struct{}), release: make(chan struct{})}
	manager := androidfs.NewManagerVFS(source, nil)
	fp := NewFileSystemPanel(0, 0, 80, 24, manager)
	t.Cleanup(func() {
		if fp.CancelLoad != nil {
			fp.CancelLoad()
		}
		fp.StopLoadingAnimation()
		fp.LoadWorkerWG.Wait()
	})
	waitForLoad(t, fp)
	child := &trackedMountedVFS{NullVFS: vfs.NewNullVFS(0), parent: manager}
	fp.Vfs = child
	fp.ReadDirectory()
	waitForLoad(t, fp)
	source.block.Store(true)
	fp.ProviderEntryName = "cached-phone"
	pf := &PanelsFrame{}
	if !pf.NavigateToPath(fp, "..") {
		t.Fatal("parent navigation failed")
	}
	select {
	case <-source.started:
	case <-time.After(2 * time.Second):
		t.Fatal("discovery did not start")
	}
	if got := fp.GetRawSelectedName(); got != "cached-phone" {
		t.Fatalf("device list while ADB is blocked = %q, want cached-phone", got)
	}
	if !fp.IsLoading || !fp.semanticLoading() {
		t.Fatal("cached list does not report loading")
	}
	close(source.release)
	waitForLoad(t, fp)
	if got := fp.GetRawSelectedName(); got != "fresh-phone" {
		t.Fatalf("refreshed device = %q", got)
	}
	if fp.IsLoading || fp.semanticLoading() {
		t.Fatal("loading remained after discovery")
	}
}
