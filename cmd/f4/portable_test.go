package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_PortableProfile(t *testing.T) {
	tmpDir := t.TempDir()

	// Имитируем путь исполняемого файла в тестовой директории
	origExeFunc := osExecutable
	origConfigDir := GetF4ConfigDir()
	origPortable := cachedF4Portable
	t.Cleanup(func() {
		osExecutable = origExeFunc
		resetConfigDirForTest()
		cachedF4ConfigDir = origConfigDir
		cachedF4Portable = origPortable
		configDirOnce.Do(func() {})
	})
	mockExe := filepath.Join(tmpDir, "f4.exe")
	if err := os.WriteFile(mockExe, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
	osExecutable = func() (string, error) {
		return mockExe, nil
	}

	// 1. Создаем f4.exe.ini с параметром UseSystemProfiles = 0
	iniContent := `
[General]
UseSystemProfiles = 0
`
	if err := os.WriteFile(mockExe+".ini", []byte(iniContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Сбрасываем кэш путей
	resetConfigDirForTest()

	gotDir := GetF4ConfigDir()
	wantDir := filepath.Join(tmpDir, "Profile")

	if filepath.Clean(gotDir) != filepath.Clean(wantDir) {
		t.Errorf("Expected portable profile dir %q, got %q", wantDir, gotDir)
	}
}
