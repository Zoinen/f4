//go:build windows

package svcmgr

import "testing"

// TestListServicesFromTheServiceManager asks the real Service Control
// Manager: every Windows machine has services, and the RPC service (RpcSs)
// is always running.
func TestListServicesFromTheServiceManager(t *testing.T) {
	services, err := listServices("")
	if err != nil {
		t.Fatal(err)
	}
	if len(services) < 10 {
		t.Fatalf("only %d services listed", len(services))
	}
	found := false
	for _, s := range services {
		if s.Name == "" {
			t.Fatalf("a service without a name: %+v", s)
		}
		if s.Name == "RpcSs" {
			found = true
			if s.State != stateRunning || s.PID == 0 {
				t.Errorf("RpcSs = %+v, want running with a process id", s)
			}
		}
	}
	if !found {
		t.Error("RpcSs is not in the list")
	}
}
