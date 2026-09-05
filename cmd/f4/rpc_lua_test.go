package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// mockHostAPI captures logs from the plugin for verification
type mockHostAPI struct {
	coreAPI
	logs []string
}

func (m *mockHostAPI) Log(msg string) {
	m.logs = append(m.logs, msg)
}

func TestLuaPluginIntegration(t *testing.T) {
	// 1. Environmental Check: do we even have Lua?
	_, err := exec.LookPath("lua")
	if err != nil {
		t.Skip("Skipping Lua integration test: 'lua' interpreter not found in PATH")
	}

	// 2. Dependency Check: does Lua have MessagePack?
	checkCmd := exec.Command("lua", "-e", "require('MessagePack')")
	if err := checkCmd.Run(); err != nil {
		t.Skip("Skipping Lua integration test: Lua module 'MessagePack' not installed")
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// 3. Initialize the real RPC plugin pointing to the dummy script
	pluginPath := filepath.Join(moduleRootDir(t), "plugins", "dummy_lua", "plugin.lua")
	p := NewRPCPlugin(pluginPath)
	host := &mockHostAPI{}

	// We need to run Init in a timeout because it's a blocking RPC call
	done := make(chan error, 1)
	go func() {
		done <- p.Init(host)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Failed to initialize Lua plugin: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for Lua plugin initialization")
	}

	// 4. Verify Handshake results
	foundDrive := false
	for _, drv := range DriveRegistry {
		// The dummy plugin registers "Lua Virtual Drive"
		if strings.Contains(drv.Name, "Lua") {
			foundDrive = true
			break
		}
	}
	if !foundDrive {
		t.Error("Lua plugin failed to register its VFS drive in the global registry")
	}

	// 5. Verify VFS Communication
	luaVfs := NewRPCVFS(p.sess, "Lua Virtual Drive")
	var items []vfs.VFSItem
	err = luaVfs.ReadDir(context.Background(), "/", func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	})

	if err != nil {
		t.Fatalf("VFS.ReadDir failed over Lua RPC: %v", err)
	}

	// Check if we got our virtual files from plugin.lua
	foundReadme := false
	for _, itm := range items {
		if itm.Name == "readme.txt" {
			foundReadme = true
			break
		}
	}

	if !foundReadme {
		t.Errorf("VFS.ReadDir didn't return expected files. Got: %v", items)
	}

	// 6. Verify Host Callbacks (Logging)
	foundLog := false
	for _, l := range host.logs {
		if strings.Contains(l, "Lua Plugin: Initializing") {
			foundLog = true
			break
		}
	}
	if !foundLog {
		t.Error("Host.Log callback from Lua was not captured by Go side")
	}

	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}
