package plughost

import (
	"testing"
)

func TestSettingsPermissionWriteFailureDoesNotRevokeInMemory(t *testing.T) {
	s := &PermissionStore{path: t.TempDir(), granted: map[string]map[string]string{"plugin": {"ffi": "allow"}}}
	if err := s.Revoke("plugin", "ffi"); err == nil {
		t.Fatal("expected write failure")
	}
	if value, ok := s.Decision("plugin", "ffi"); !ok || value != "allow" {
		t.Fatal("failed write revoked runtime permission")
	}
}
