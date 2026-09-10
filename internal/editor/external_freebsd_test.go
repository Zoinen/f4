//go:build freebsd

package editor

import (
	"os/exec"
	"testing"
)

func TestConfigureExternalEditorProcessInheritsFreeBSDTTYSession(t *testing.T) {
	cmd := exec.Command("true")
	ConfigureExternalEditorProcess(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatalf("FreeBSD editor process attributes = %+v, want inherited terminal session", cmd.SysProcAttr)
	}
}
