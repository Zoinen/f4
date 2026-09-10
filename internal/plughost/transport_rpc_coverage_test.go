package plughost

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

type rpcCoverageRegistration struct {
	called int
}

func (r *rpcCoverageRegistration) Unregister() {
	r.called++
}

func TestRPCPluginTransportConstructorsAndIdentity(t *testing.T) {
	plugin := NewRPCPlugin("plugin --flag")
	if plugin.path != "plugin --flag" || plugin.dir != "" {
		t.Fatalf("NewRPCPlugin = %#v", plugin)
	}
	if plugin.GetName() != "plugin --flag (RPC)" {
		t.Fatalf("GetName() = %q", plugin.GetName())
	}

	plugin.SetPermissionIdentity(PluginIdentity{
		Key:      "catalog-id",
		Title:    "Catalog title",
		Declared: map[string]string{PermissionNative: "run the helper"},
	})
	identity := plugin.permissionIdentity()
	if identity.Key != "catalog-id" || identity.Title != "Catalog title" || identity.Declared[PermissionNative] != "run the helper" {
		t.Fatalf("permissionIdentity() = %#v", identity)
	}

	ringPlugin := NewRPCPlugRing("/plugins/catalog-id", "plugin --flag")
	if ringPlugin.dir != "/plugins/catalog-id" || ringPlugin.path != "plugin --flag" {
		t.Fatalf("NewRPCPlugRing = %#v", ringPlugin)
	}

	fallback := NewRPCPlugin("/opt/f4/plugin")
	if identity := fallback.permissionIdentity(); identity.Key != fallback.path {
		t.Fatalf("fallback identity key = %q, want %q", identity.Key, fallback.path)
	}
}

func TestRPCPluginInitRejectsEmptyEntrypoint(t *testing.T) {
	err := NewRPCPlugin(" \t\n").Init(nil)
	if err == nil || err.Error() != "empty entrypoint" {
		t.Fatalf("Init error = %v, want empty entrypoint", err)
	}
}

func TestRPCPluginCloseUnregistersContribution(t *testing.T) {
	registration := &rpcCoverageRegistration{}
	plugin := &RPCPlugin{registration: registration}

	if err := plugin.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !plugin.closing || plugin.registration != nil || registration.called != 1 {
		t.Fatalf("after Close: closing=%v registration=%#v unregisters=%d", plugin.closing, plugin.registration, registration.called)
	}
	if err := plugin.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if registration.called != 1 {
		t.Fatalf("second Close called Unregister %d times", registration.called)
	}
}

func TestRPCPluginCloseKillsProcess(t *testing.T) {
	if os.Getenv("F4_RPC_PLUGIN_HELPER") == "1" {
		time.Sleep(time.Minute)
		return
	}

	// #nosec G204 G702 -- the test deliberately starts this test binary as a helper.
	cmd := exec.Command(os.Args[0], "-test.run=TestRPCPluginCloseKillsProcess")
	cmd.Env = append(os.Environ(), "F4_RPC_PLUGIN_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	plugin := &RPCPlugin{cmd: cmd}
	if err := plugin.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("Close() did not wait for the killed process")
	}
}
