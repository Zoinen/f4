package plughost

import (
	"context"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

func TestRPCPlugin_Handshake(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	pluginSess.Register("Plugin.Init", func(data msgpack.RawMessage) (any, error) {
		return map[string]any{"Drives": []string{"TestDrive"}}, nil
	})

	api := newLuaTestHostAPI()
	p := &RPCPlugin{path: "test", sess: coreSess, api: api}

	type PluginInitRes struct{ Drives []string }
	var res PluginInitRes
	err := p.sess.Call("Plugin.Init", nil, &res)

	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if len(res.Drives) != 1 || res.Drives[0] != "TestDrive" {
		t.Errorf("Unexpected drives: %v", res.Drives)
	}
}

func TestRPCPlugin_VFS_Proxy(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	wrapper := &rpcFileWrapper{
		sess: coreSess,
		id:   1,
		size: 100,
	}

	pluginSess.Register("VFS.ReadAt", func(data msgpack.RawMessage) (any, error) {
		var req ReadAtReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		if req.ID == 1 {
			return []byte("data"), nil
		}
		return nil, nil
	})

	buf := make([]byte, 4)
	n, err := wrapper.ReadAt(context.Background(), buf, 0)
	if err != nil {
		t.Errorf("Proxy ReadAt failed: %v", err)
	}
	if n != 4 || string(buf) != "data" {
		t.Errorf("Data corruption in RPC proxy: %q", string(buf))
	}
}

func TestRPCPlugin_Highlighter_Proxy(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	h := &rpcHighlighter{transport: coreSess}

	pluginSess.Register("VFS.Highlight", func(data msgpack.RawMessage) (any, error) {
		return HighlightRes{Attrs: []uint64{42, 42}, Next: "state2"}, nil
	})

	attrs, next := h.Highlight("hi", nil, 0)
	if len(attrs) != 2 || attrs[0] != 42 || next != "state2" {
		t.Errorf("Highlighter proxy failed: attrs=%v, next=%v", attrs, next)
	}
}

func TestRPCPlugin_Hotkey_Proxy(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	hotkeyTriggered := false
	pluginSess.Register("Plugin.OnHotkey", func(data msgpack.RawMessage) (any, error) {
		hotkeyTriggered = true
		return nil, nil
	})

	// Simulate core calling the hotkey callback
	req := HotkeyReq{VK: 0x41, Mods: 0}
	err := coreSess.Call("Plugin.OnHotkey", req, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	if !hotkeyTriggered {
		t.Error("Hotkey proxy failed to reach plugin")
	}
}

func TestRPCPlugin_Progress_Proxy(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	updateMsg := ""
	updatePct := -1

	coreSess.Register("Host.UpdateProgress", func(data msgpack.RawMessage) (any, error) {
		var req ProgressUpdateReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		updateMsg = req.Msg
		updatePct = req.Percent
		return nil, nil
	})

	// Plugin sends progress update to core
	err := pluginSess.Call("Host.UpdateProgress", ProgressUpdateReq{Msg: "working", Percent: 50}, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	if updateMsg != "working" || updatePct != 50 {
		t.Errorf("Progress proxy failed: msg=%q, pct=%d", updateMsg, updatePct)
	}
}
func TestRPCPlugin_InputBox_Proxy(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	coreSess.Register("Host.InputBox", func(data msgpack.RawMessage) (any, error) {
		return "user_input", nil
	})

	var res string
	err := pluginSess.Call("Host.InputBox", InputBoxReq{Title: "T", Prompt: "P"}, &res)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if res != "user_input" {
		t.Errorf("Expected 'user_input', got %q", res)
	}
}

func TestRPCPlugin_SetAttributes_Proxy(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	v := &RPCVFS{sess: coreSess, driveName: "TestDrive"}
	item := vfs.VFSItem{Name: "file", UnixMode: 0644}

	var capturedReq SetAttrReq
	pluginSess.Register("VFS.SetAttributes", func(data msgpack.RawMessage) (any, error) {
		if err := msgpack.Unmarshal(data, &capturedReq); err != nil {
			return nil, err
		}
		return nil, nil
	})

	err := v.SetAttributes(context.Background(), "/path/file", item)
	if err != nil {
		t.Fatalf("SetAttributes failed: %v", err)
	}
	if capturedReq.Item.UnixMode != 0644 || capturedReq.Path != "/path/file" {
		t.Errorf("Data corruption in SetAttributes proxy: %+v", capturedReq)
	}
}
func TestRPCPlugin_Progress_Cancellation(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	ctx, cancel := context.WithCancel(context.Background())
	// Inject a mock task context into core side manually
	coreSess.Register("Host.IsProgressCancelled", func(data msgpack.RawMessage) (any, error) {
		return ctx.Err() != nil, nil
	})

	var isCancelled bool
	// 1. Initially not cancelled
	err := pluginSess.Call("Host.IsProgressCancelled", nil, &isCancelled)
	if err != nil || isCancelled {
		t.Errorf("Expected not cancelled, err: %v", err)
	}

	// 2. Cancel and check again
	cancel()
	_ = pluginSess.Call("Host.IsProgressCancelled", nil, &isCancelled)
	if !isCancelled {
		t.Error("Plugin failed to detect cancellation through RPC")
	}
}
func TestRPCPlugin_NativePermissionDenied(t *testing.T) {
	tmpDir := t.TempDir()
	_ = config.GetF4ConfigDir()
	_ = PluginPermissions()
	oldConfigDir := config.CachedF4ConfigDir
	oldPermissionStore := pluginPermissionStore
	config.CachedF4ConfigDir = tmpDir
	t.Cleanup(func() {
		config.CachedF4ConfigDir = oldConfigDir
		pluginPermissionStore = oldPermissionStore
	})

	pluginPermissionStore = LoadPermissionStore(DefaultPermissionStorePath())

	pluginPath := "denied_plugin"
	err := PluginPermissions().Remember(pluginPath, PermissionNative, PermissionDeny)
	if err != nil {
		t.Fatalf("Failed to seed permission: %v", err)
	}

	p := NewRPCPlugin(pluginPath)
	err = p.Init(newLuaTestHostAPI())
	if err == nil {
		t.Fatal("Expected Init to fail due to Denied Native permission")
	}
	if !strings.Contains(err.Error(), "is not allowed to run a program of its own") {
		t.Errorf("Unexpected error message: %v", err)
	}
}
