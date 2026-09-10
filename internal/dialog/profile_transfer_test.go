package dialog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsProfileTransferCopyAndMove(t *testing.T) {
	for _, move := range []bool{false, true} {
		t.Run(map[bool]string{false: "copy", true: "move"}[move], func(t *testing.T) {
			base := t.TempDir()
			src, dst := filepath.Join(base, "source"), filepath.Join(base, "target")
			if err := os.MkdirAll(src, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(src, "settings.ini"), []byte("preserved"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := TransferProfile(filepath.Join(base, "f4.ini"), src, dst, true, move); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dst, "settings.ini"))
			if err != nil || string(data) != "preserved" {
				t.Fatalf("destination=%q error=%v", data, err)
			}
			_, err = os.Stat(src)
			if move != os.IsNotExist(err) {
				t.Fatalf("source after transfer: %v", err)
			}
		})
	}
}
func TestSettingsProfileTransferConflictRetainsSource(t *testing.T) {
	base := t.TempDir()
	src, dst := filepath.Join(base, "source"), filepath.Join(base, "target")
	for _, dir := range []string{src, dst} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.ini"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ini := filepath.Join(base, "f4.ini")
	if err := TransferProfile(ini, src, dst, true, true); err == nil {
		t.Fatal("conflicting move succeeded")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ini); !os.IsNotExist(err) {
		t.Fatal("profile selection changed on failure")
	}
}
