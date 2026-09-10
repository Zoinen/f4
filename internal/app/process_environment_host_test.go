package app

import (
	"github.com/unxed/f4/vfs"
	"testing"
)

func TestCoreAPIProcessEnvironmentHost(t *testing.T) {
	api := &CoreAPI{}
	if snapshot := api.SnapshotProcessEnvironment(); snapshot.Variables == nil {
		t.Fatal("SnapshotProcessEnvironment returned a nil variable list")
	}
	if _, err := api.ApplyProcessEnvironment([]vfs.ProcessEnvironmentChange{{Name: "invalid-name"}}); err == nil {
		t.Fatal("ApplyProcessEnvironment accepted an invalid variable name")
	}
}
