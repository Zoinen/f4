package main

import (
	"os"
	"path/filepath"
	"strings"
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

func TestResolveProfileDir_ProfileKey(t *testing.T) {
	exeDir := t.TempDir()
	t.Setenv("F4_GENERAL_USE_SYSTEM_PROFILES", "")
	t.Setenv("F4_GENERAL_PROFILE", "")
	t.Setenv("F4TESTVAR", filepath.Join(exeDir, "fromenv"))

	cases := []struct {
		name     string
		ini      string
		want     string
		portable bool
	}{
		{"default portable", "[General]\nUseSystemProfiles=0\n", filepath.Join(exeDir, "Profile"), true},
		{"relative", "[General]\nUseSystemProfiles=0\nProfile=data/cfg\n", filepath.Join(exeDir, "data", "cfg"), true},
		{"f4home percent", "[General]\nUseSystemProfiles=0\nProfile=%F4HOME%/Prof\n", filepath.Join(exeDir, "Prof"), true},
		{"f4home dollar", "[General]\nUseSystemProfiles=0\nProfile=${F4HOME}/Prof2\n", filepath.Join(exeDir, "Prof2"), true},
		{"other env", "[General]\nUseSystemProfiles=0\nProfile=%F4TESTVAR%\n", filepath.Join(exeDir, "fromenv"), true},
		{"profile ignored when system", "[General]\nUseSystemProfiles=1\nProfile=%F4HOME%/Prof\n", "", false},
		{"bom and crlf", "\xef\xbb\xbf[General]\r\nUseSystemProfiles = 0\r\n", filepath.Join(exeDir, "Profile"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, portable := resolveProfileDir(exeDir, ParseIni(strings.NewReader(tc.ini)))
			if portable != tc.portable {
				t.Fatalf("portable = %v, want %v", portable, tc.portable)
			}
			if tc.portable && got != tc.want {
				t.Errorf("dir = %q, want %q", got, tc.want)
			}
			if !tc.portable && strings.HasPrefix(got, exeDir) {
				t.Errorf("system profile %q must not live under exeDir", got)
			}
		})
	}
}

func TestGetF4ConfigDir_ExportsF4HOME(t *testing.T) {
	tmpDir := setupPortableIni(t, "0")
	t.Setenv("F4HOME", "")
	_ = GetF4ConfigDir()
	if got := os.Getenv("F4HOME"); filepath.Clean(got) != filepath.Clean(tmpDir) {
		t.Errorf("F4HOME = %q, want %q", got, tmpDir)
	}
}

func TestPortableIniPath_PrefersExeIni(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "f4-gui.exe")
	if got, want := portableIniPath(exe), filepath.Join(dir, portableIniName); got != want {
		t.Errorf("without exe.ini: %q, want %q", got, want)
	}
	if err := os.WriteFile(exe+".ini", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got, want := portableIniPath(exe), exe+".ini"; got != want {
		t.Errorf("with exe.ini: %q, want %q", got, want)
	}
}

func TestSetPortableMode_RoundTripKeepsOtherKeys(t *testing.T) {
	iniPath := filepath.Join(t.TempDir(), portableIniName)

	if err := setPortableMode(iniPath, true); err != nil {
		t.Fatal(err)
	}
	if got := LoadIni(iniPath).GetString("General", "UseSystemProfiles", ""); got != "0" {
		t.Fatalf("fresh file: UseSystemProfiles = %q, want 0", got)
	}
	data, _ := os.ReadFile(iniPath)
	if strings.HasPrefix(string(data), "\n") {
		t.Errorf("fresh file starts with a blank line: %q", data)
	}

	// A hand-edited file with a comment, a custom profile and CRLF endings.
	custom := "; keep me\r\n[General]\r\nUseSystemProfiles = 0\r\nProfile = %F4HOME%\\Prof\r\n"
	if err := os.WriteFile(iniPath, []byte(custom), 0600); err != nil {
		t.Fatal(err)
	}
	if err := setPortableMode(iniPath, false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(iniPath)
	text := string(data)
	ini := LoadIni(iniPath)
	if got := ini.GetString("General", "UseSystemProfiles", ""); got != "1" {
		t.Errorf("UseSystemProfiles = %q, want 1", got)
	}
	if got := ini.GetString("General", "Profile", ""); got != `%F4HOME%\Prof` {
		t.Errorf("Profile lost: %q", got)
	}
	if !strings.Contains(text, "; keep me") || !strings.Contains(text, "\r\n") {
		t.Errorf("comment or line endings lost: %q", text)
	}
}

func TestCopyProfileDir_NoClobberSkipsCrashes(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	mk := func(root, rel, body string) {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	mk(src, "settings.ini", "src")
	mk(src, "Macros/scripts/a.lua", "lua")
	mk(src, "crashes/1.log", "boom")
	mk(dst, "settings.ini", "dst")

	if err := copyProfileDir(src, dst); err != nil {
		t.Fatal(err)
	}
	read := func(rel string) string {
		b, _ := os.ReadFile(filepath.Join(dst, rel))
		return string(b)
	}
	if got := read("settings.ini"); got != "dst" {
		t.Errorf("existing file overwritten: %q", got)
	}
	if got := read("Macros/scripts/a.lua"); got != "lua" {
		t.Errorf("nested file not copied: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "crashes")); !os.IsNotExist(err) {
		t.Errorf("crash logs must not be copied")
	}
	if err := copyProfileDir(src, filepath.Join(src, "Profile")); err == nil {
		t.Errorf("copying a profile into itself must fail")
	}
	if err := copyProfileDir(filepath.Join(src, "missing"), dst); err != nil {
		t.Errorf("missing source is not an error: %v", err)
	}
}

func TestEnsureProfileLayout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Profile")
	if err := ensureProfileLayout(dir); err != nil {
		t.Fatal(err)
	}
	for _, sub := range portableProfileSubdirs {
		if st, err := os.Stat(filepath.Join(dir, sub)); err != nil || !st.IsDir() {
			t.Errorf("%s missing", sub)
		}
	}
}
