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
	mockExe := filepath.Join(tmpDir, "f4.exe")
	os.WriteFile(mockExe, []byte(""), 0755)
	osExecutable = func() (string, error) {
		return mockExe, nil
	}
	defer func() { osExecutable = origExeFunc }()

	// 1. Создаем f4.exe.ini с параметром UseSystemProfiles = 0
	iniContent := `
[General]
UseSystemProfiles = 0
`
	os.WriteFile(mockExe+".ini", []byte(iniContent), 0644)

	// Сбрасываем кэш путей
	resetConfigDirForTest()

	gotDir := GetF4ConfigDir()
	wantDir := filepath.Join(tmpDir, "Profile")

	if filepath.Clean(gotDir) != filepath.Clean(wantDir) {
		t.Errorf("Expected portable profile dir %q, got %q", wantDir, gotDir)
	}

	// Очищаем состояние для последующих тестов
	resetConfigDirForTest()
}
