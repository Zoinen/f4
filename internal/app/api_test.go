package app

import (
	"context"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"testing"
	"time"
)

type mockVFSProvider struct{}

func (m *mockVFSProvider) Name() string  { return "mock-vfs-provider" }
func (m *mockVFSProvider) Priority() int { return 1 }
func (m *mockVFSProvider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	return path == "api-test-path"
}

type mockURIProvider struct{}

func (*mockURIProvider) Scheme() string { return "core-api-uri-test" }
func (*mockURIProvider) OpenURI(context.Context, vfs.VFS, string) (vfs.VFS, error) {
	return nil, nil
}
func (m *mockVFSProvider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	return nil, nil
}

type mockHighlighter struct{}

func (m *mockHighlighter) Highlight(line string, prev any, base uint64) ([]uint64, any) {
	return nil, nil
}

type mockHighlighterProvider struct{}

func (m *mockHighlighterProvider) Name() string { return "mock-highlighter" }
func (m *mockHighlighterProvider) Match(filename, content string) bool {
	return filename == "api-test.mock"
}
func (m *mockHighlighterProvider) Create(filename, content string) vtui.Highlighter {
	return &mockHighlighter{}
}

func TestCoreAPI_GetVersion(t *testing.T) {
	api := &CoreAPI{}
	ver := api.GetVersion()
	if ver == "" {
		t.Errorf("GetVersion returned empty string")
	}
}

func TestCoreAPI_Log(t *testing.T) {
	api := &CoreAPI{}
	// Should not panic
	api.Log("test log message")
}

func TestCoreAPI_Message(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	api := &CoreAPI{}
	api.Message("api test message")

	// Process tasks
	timeout := time.After(1 * time.Second)
	found := false
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if vtui.FrameManager.GetTopFrameType() == vtui.TypeDialog {
				found = true
				break Loop
			}
		case <-timeout:
			break Loop
		}
	}

	if !found {
		t.Error("api.Message did not result in a dialog being pushed to FrameManager")
	}

	// Clean up dialog
	vtui.FrameManager.Pop()
}

func TestCoreAPI_Registrations(t *testing.T) {
	api := &CoreAPI{}
	restoreDrives := sysinfo.SnapshotDrives()
	restorePlugins := plughost.SnapshotPluginRegistries()
	t.Cleanup(func() {
		restoreDrives()
		restorePlugins()
	})

	// 1. RegisterVFSProvider
	p := &mockVFSProvider{}
	api.RegisterVFSProvider(p)
	t.Cleanup(func() { vfs.UnregisterProvider(p) })
	found := vfs.FindProvider(context.Background(), nil, "api-test-path")
	if found != p {
		t.Error("VFS provider was not registered correctly")
	}

	// 2. RegisterHighlighter
	hp := &mockHighlighterProvider{}
	api.RegisterHighlighter(hp)
	hl := vtui.GetHighlighter("api-test.mock", "")
	if _, ok := hl.(*mockHighlighter); !ok {
		t.Error("Highlighter was not registered correctly")
	}

	// 3. sysinfo.RegisterDrive
	initialLen := len(sysinfo.Drives())
	api.RegisterDrive("MockDrive", func() vfs.VFS { return nil })
	if drives := sysinfo.Drives(); len(drives) != initialLen+1 || drives[len(drives)-1].Name != "MockDrive" {
		t.Error("Drive was not registered correctly")
	}

	// 4. RegisterURIProvider
	up := &mockURIProvider{}
	if err := api.RegisterURIProvider(up); err != nil {
		t.Fatalf("RegisterURIProvider: %v", err)
	}
	t.Cleanup(func() { vfs.UnregisterURIProvider(up.Scheme()) })
	if got := vfs.FindURIProvider("CORE-API-URI-TEST://profile/path"); got != up {
		t.Error("URI provider was not registered correctly")
	}

	// 5. plughost.RegisterGlobalHotkey
	initialHotkeys := len(plughost.GlobalHotkeys)
	api.RegisterGlobalHotkey(0x41, vtinput.ShiftPressed, func(app vfs.App) {})
	if len(plughost.GlobalHotkeys) != initialHotkeys+1 || plughost.GlobalHotkeys[len(plughost.GlobalHotkeys)-1].VK != 0x41 {
		t.Error("Hotkey was not registered correctly")
	}

	// 6. plughost.RegisterPluginMenuItem
	initialPlugins := len(plughost.PluginMenuItems)
	api.RegisterPluginMenuItem("My Plugin", func(app vfs.App) {})
	if len(plughost.PluginMenuItems) != initialPlugins+1 || plughost.PluginMenuItems[len(plughost.PluginMenuItems)-1].Label != "My Plugin" {
		t.Error("Plugin menu item was not registered correctly")
	}
}
