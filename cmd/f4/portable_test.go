package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func portableSettingsMouseCoordinate(value int) int16 {
	return int16(value) // #nosec G115 -- the test dialog is inside the test screen.
}

func portableSettingsDialogButtonBounds(t *testing.T, dlg *portableSettingsDialog) (int, int) {
	t.Helper()
	minX, maxX := 0, 0
	buttonCount := 0
	for _, child := range dlg.GetChildren() {
		button, ok := child.(*vtui.Button)
		if !ok {
			continue
		}
		x1, _, x2, _ := button.GetPosition()
		if buttonCount == 0 || x1 < minX {
			minX = x1
		}
		if buttonCount == 0 || x2 > maxX {
			maxX = x2
		}
		buttonCount++
	}
	if buttonCount != 2 {
		t.Fatalf("portable settings has %d buttons, want 2", buttonCount)
	}
	return minX, maxX
}

func TestPortableSettingsDialogUsesContextHelp(t *testing.T) {
	initFrameworkActionTestScreen(t)
	tmpDir := t.TempDir()
	exe := filepath.Join(tmpDir, "f4")
	if err := os.WriteFile(exe, nil, 0600); err != nil {
		t.Fatal(err)
	}

	oldExecutable := osExecutable
	oldUserConfigDir := userConfigDir
	osExecutable = func() (string, error) { return exe, nil }
	userConfigDir = func() (string, error) { return tmpDir, nil }
	resetConfigDirForTest()
	t.Cleanup(func() {
		osExecutable = oldExecutable
		userConfigDir = oldUserConfigDir
		resetConfigDirForTest()
	})

	actionPortableSettings(nil)
	top := vtui.FrameManager.GetTopFrame()
	if top == nil {
		t.Fatal("portable settings did not open a dialog")
	}
	if got := top.GetHelp(); got != "PortableSettings" {
		t.Fatalf("portable settings help topic = %q, want PortableSettings", got)
	}
	dlg, ok := top.(*portableSettingsDialog)
	if !ok {
		t.Fatalf("portable settings frame has type %T, want *portableSettingsDialog", top)
	}
	startX1, startX2, startY2 := dlg.X1, dlg.X2, dlg.Y2
	if !dlg.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      portableSettingsMouseCoordinate(startX2),
		MouseY:      portableSettingsMouseCoordinate(startY2),
	}) {
		t.Fatal("portable settings resize corner was not handled")
	}
	dlg.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      portableSettingsMouseCoordinate(startX2 + 8),
		MouseY:      portableSettingsMouseCoordinate(startY2 + 4),
	})
	dlg.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	if dlg.X1 != startX1-8 {
		t.Errorf("portable settings left edge = %d, want %d", dlg.X1, startX1-8)
	}
	if dlg.X2 != startX2+8 {
		t.Errorf("portable settings right edge = %d, want %d", dlg.X2, startX2+8)
	}
	if dlg.X1+dlg.X2 != startX1+startX2 {
		t.Errorf("portable settings center moved from %d to %d", startX1+startX2, dlg.X1+dlg.X2)
	}
	if dlg.Y2 != startY2 {
		t.Errorf("portable settings bottom edge = %d, want fixed %d", dlg.Y2, startY2)
	}
	buttonX1, buttonX2 := portableSettingsDialogButtonBounds(t, dlg)
	if buttonX1+buttonX2 != dlg.X1+dlg.X2 {
		t.Fatalf("portable settings buttons center = %d, dialog center = %d before screen resize", buttonX1+buttonX2, dlg.X1+dlg.X2)
	}

	width := dlg.X2 - dlg.X1 + 1
	vtui.FrameManager.Resize(120, 30)
	if got := dlg.X2 - dlg.X1 + 1; got != width {
		t.Errorf("portable settings width after screen resize = %d, want %d", got, width)
	}
	if got := dlg.X1 + dlg.X2; got != 119 {
		t.Errorf("portable settings center after screen resize = %d, want 119", got)
	}
	buttonX1, buttonX2 = portableSettingsDialogButtonBounds(t, dlg)
	if got := buttonX1 + buttonX2; got != dlg.X1+dlg.X2 {
		t.Errorf("portable settings buttons center after screen resize = %d, dialog center = %d", got, dlg.X1+dlg.X2)
	}
}

func TestPortableSettingsHelpTopicIsRegistered(t *testing.T) {
	engine := vtui.NewHelpEngine(&memoryHelpVFS{files: map[string]string{
		"help.hlf": defaultHelpData,
	}})
	if err := engine.LoadFile("help.hlf"); err != nil {
		t.Fatal(err)
	}
	topic := engine.GetTopic("PortableSettings")
	if topic == nil {
		t.Fatal("PortableSettings help topic is missing")
	}
	content := strings.Join(topic.Lines, "\n")
	for _, want := range []string{
		"UseSystemProfiles=0",
		"f4.exe.ini",
		"f4.example.ini",
		"Macros/scripts",
		"logs/",
		"crashes/",
		"Restart f4",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("PortableSettings help does not mention %q", want)
		}
	}
}

func TestConfig_PortableProfile(t *testing.T) {
	tmpDir := t.TempDir()

	// Имитируем путь исполняемого файла в тестовой директории
	origExeFunc := osExecutable
	t.Cleanup(func() {
		osExecutable = origExeFunc
		// Do not restore cachedF4ConfigDir by hand: the preceding test may
		// have left configDirOnce completed with an empty cache.  A fresh
		// detection is the only valid state after changing osExecutable.
		resetConfigDirForTest()
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

func TestTransferProfileDir_MoveKeepsSourceOnTransferFailure(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(src, "Profile")
	if err := os.MkdirAll(src, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(src, "settings.ini")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := moveProfileDir(src, dst); err == nil {
		t.Fatal("copying a profile into itself should fail before a move can remove the source")
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "keep" {
		t.Fatalf("source profile changed after failed transfer: %q, %v", got, err)
	}
}

func TestMoveProfileDir_RejectsDestinationConflict(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	for root, contents := range map[string]string{src: "source", dst: "destination"} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "settings.ini"), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(src, "a.ini"), []byte("must not be copied"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := moveProfileDir(src, dst); err == nil {
		t.Fatal("moving over an existing profile file should fail")
	}
	if got, err := os.ReadFile(filepath.Join(src, "settings.ini")); err != nil || string(got) != "source" {
		t.Fatalf("source profile changed after destination conflict: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "settings.ini")); err != nil || string(got) != "destination" {
		t.Fatalf("destination profile changed after conflict: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "a.ini")); !os.IsNotExist(err) {
		t.Fatalf("move copied files before discovering destination conflict: %v", err)
	}
}

func TestTransferProfileDir_MoveIncludesCrashLogs(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	dst := filepath.Join(t.TempDir(), "dst")
	crash := filepath.Join(src, "crashes", "1.log")
	if err := os.MkdirAll(filepath.Dir(crash), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(crash, []byte("diagnostic"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := moveProfileDir(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dst, "crashes", "1.log")); err != nil || string(got) != "diagnostic" {
		t.Fatalf("crash log was not transferred for Move: %q, %v", got, err)
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
