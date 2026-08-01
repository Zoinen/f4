package main

import (
	"context"
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
	api := &coreAPI{}
	ver := api.GetVersion()
	if ver == "" {
		t.Errorf("GetVersion returned empty string")
	}
}

func TestCoreAPI_Log(t *testing.T) {
	api := &coreAPI{}
	// Should not panic
	api.Log("test log message")
}

func TestCoreAPI_Message(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	api := &coreAPI{}
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
	api := &coreAPI{}

	// 1. RegisterVFSProvider
	p := &mockVFSProvider{}
	api.RegisterVFSProvider(p)
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

	// 3. RegisterDrive
	initialLen := len(DriveRegistry)
	api.RegisterDrive("MockDrive", func() vfs.VFS { return nil })
	if len(DriveRegistry) != initialLen+1 || DriveRegistry[len(DriveRegistry)-1].Name != "MockDrive" {
		t.Error("Drive was not registered correctly")
	}

	// 4. RegisterGlobalHotkey
	initialHotkeys := len(GlobalHotkeys)
	api.RegisterGlobalHotkey(0x41, vtinput.ShiftPressed, func(app vfs.App) {})
	if len(GlobalHotkeys) != initialHotkeys+1 || GlobalHotkeys[len(GlobalHotkeys)-1].VK != 0x41 {
		t.Error("Hotkey was not registered correctly")
	}

	// 5. RegisterPluginMenuItem
	initialPlugins := len(PluginMenuItems)
	api.RegisterPluginMenuItem("My Plugin", func(app vfs.App) {})
	if len(PluginMenuItems) != initialPlugins+1 || PluginMenuItems[len(PluginMenuItems)-1].Label != "My Plugin" {
		t.Error("Plugin menu item was not registered correctly")
	}
}
