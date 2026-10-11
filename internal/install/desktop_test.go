package install

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/gui/assets/icon"
)

func TestDataHome(t *testing.T) {
	get := func(v string) func(string) string {
		return func(k string) string {
			if k == "XDG_DATA_HOME" {
				return v
			}
			return ""
		}
	}
	if got := DataHome("/home/u", get("")); got != filepath.Join("/home/u", ".local", "share") {
		t.Errorf("default = %q", got)
	}
	abs := filepath.Join(t.TempDir(), "data")
	if got := DataHome("/home/u", get(abs)); got != abs {
		t.Errorf("XDG_DATA_HOME = %q", got)
	}
	if got := DataHome("/home/u", get("relative/dir")); got != filepath.Join("/home/u", ".local", "share") {
		t.Errorf("a relative XDG_DATA_HOME must be ignored, got %q", got)
	}
}

func TestExecQuote(t *testing.T) {
	cases := map[string]string{
		"/home/u/.local/bin/f4":   "/home/u/.local/bin/f4",
		"/opt/my apps/f4":         `"/opt/my apps/f4"`,
		"/opt/50%/f4":             "/opt/50%%/f4",
		`/opt/a"b/f4`:             `"/opt/a\"b/f4"`,
		"/opt/$HOME/f4":           `"/opt/\$HOME/f4"`,
		`/opt/back` + `\` + `/f4`: `"/opt/back\\\\/f4"`,
	}
	for in, want := range cases {
		if got := execQuote(in); got != want {
			t.Errorf("execQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

// The launcher this package writes must not drift from the one the release
// archives carry: the two differ only in the executable's path.
func TestDesktopEntryMatchesThePackagedOne(t *testing.T) {
	packaged, err := os.ReadFile(filepath.Join("..", "..", "packaging", "linux", DesktopFileName))
	if err != nil {
		t.Fatal(err)
	}
	strip := func(text string) string {
		var keep []string
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "Exec=") || strings.HasPrefix(line, "TryExec=") || strings.HasPrefix(line, ManagedKey+"=") {
				continue
			}
			keep = append(keep, line)
		}
		return strings.Join(keep, "\n")
	}
	got := DesktopEntry("/x/f4")
	if strip(got) != strip(string(packaged)) {
		t.Errorf("the generated launcher differs from packaging/linux/%s:\n%s\nvs\n%s", DesktopFileName, got, packaged)
	}
	if !strings.Contains(got, "Exec=/x/f4 --gui\n") || !strings.Contains(got, "TryExec=/x/f4\n") {
		t.Errorf("Exec/TryExec lines missing:\n%s", got)
	}
}

func TestInstallDesktopWritesLauncherAndIcons(t *testing.T) {
	data := t.TempDir()
	written, err := InstallDesktop(data, "/opt/f4 dir/f4")
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(data, "applications", DesktopFileName)
	text, err := os.ReadFile(launcher) // #nosec G304 -- a path inside the test's own temp dir.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), `Exec="/opt/f4 dir/f4" --gui`) || !strings.Contains(string(text), "StartupWMClass=org.unxed.f4") {
		t.Errorf("launcher:\n%s", text)
	}
	for _, rel := range []string{
		filepath.Join("icons", "hicolor", "scalable", "apps", IconName+".svg"),
		filepath.Join("icons", "hicolor", "48x48", "apps", IconName+".png"),
		filepath.Join("icons", "hicolor", "512x512", "apps", IconName+".png"),
	} {
		info, err := os.Stat(filepath.Join(data, rel))
		if err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", rel, err)
		}
	}
	if len(written) != 2+len(pngSizesForTest()) {
		t.Errorf("wrote %d files: %v", len(written), written)
	}
	// Again: the same files, no leftovers.
	if _, err := InstallDesktop(data, "/opt/f4 dir/f4"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(data, "applications"))
	if len(entries) != 1 {
		t.Errorf("applications/ holds %d entries after a second install", len(entries))
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(launcher); info.Mode().Perm() != 0o644 {
			t.Errorf("launcher mode = %v", info.Mode().Perm())
		}
	}
}

func TestInstallDesktopReportsAnUnwritableDirectory(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallDesktop(blocker, "/x/f4"); err == nil {
		t.Error("no error when the data directory is a file")
	}
}

func TestRunDesktopCLIInstallsIntoTheUsersDataDirectory(t *testing.T) {
	if !SupportsDesktop() {
		t.Skip("no freedesktop launchers on this OS")
	}
	home := t.TempDir()
	oldHome, oldGetenv := UserHomeDir, Getenv
	UserHomeDir = func() (string, error) { return home, nil }
	Getenv = func(string) string { return "" }
	t.Cleanup(func() { UserHomeDir, Getenv = oldHome, oldGetenv })
	if code := RunDesktopCLI(filepath.Join(home, "bin", "f4")); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "share", "applications", DesktopFileName)); err != nil {
		t.Error(err)
	}
}

func pngSizesForTest() []int { return icon.PNGSizes }

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- a path inside the test's own temp dir.
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEnsureDesktopFromScratchAndWhenNothingChanged(t *testing.T) {
	data := t.TempDir()
	written, err := EnsureDesktop(data, "/opt/f4/f4")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 1+1+len(icon.PNGSizes) {
		t.Errorf("first run wrote %d files: %v", len(written), written)
	}
	if !strings.Contains(readFile(t, launcherPath(data)), ManagedKey+"=true\n") {
		t.Error("the launcher is not marked as managed")
	}
	written, err = EnsureDesktop(data, "/opt/f4/f4")
	if err != nil || len(written) != 0 {
		t.Errorf("second run: %v, %v", written, err)
	}
}

func TestEnsureDesktopLeavesTheUsersOwnLauncherButFixesIcons(t *testing.T) {
	data := t.TempDir()
	own := "[Desktop Entry]\nType=Application\nName=f4\nExec=/my/f4 --gui=x11\nIcon=io.github.unxed.f4\n"
	if err := os.MkdirAll(filepath.Join(data, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath(data), []byte(own), 0o644); err != nil { // #nosec G306 -- test file.
		t.Fatal(err)
	}
	written, err := EnsureDesktop(data, "/opt/f4/f4")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, launcherPath(data)); got != own {
		t.Errorf("the user's launcher was changed:\n%s", got)
	}
	if len(written) != 1+len(icon.PNGSizes) {
		t.Errorf("wrote %d files, want the icons only: %v", len(written), written)
	}
}

func TestEnsureDesktopRefreshesAStaleIconAndKeepsALiveExecutable(t *testing.T) {
	data := t.TempDir()
	live := filepath.Join(t.TempDir(), "f4")
	if err := os.WriteFile(live, []byte("x"), 0o755); err != nil { // #nosec G306 -- test file.
		t.Fatal(err)
	}
	if _, err := InstallDesktop(data, live); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(data, "icons", "hicolor", "48x48", "apps", IconName+".png")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil { // #nosec G306 -- test file.
		t.Fatal(err)
	}
	// An older text of the launcher, still pointing at the live executable.
	old := strings.Replace(DesktopEntry(live), "Comment=Far Manager / far2l-style file manager", "Comment=old", 1)
	if err := os.WriteFile(launcherPath(data), []byte(old), 0o644); err != nil { // #nosec G306 -- test file.
		t.Fatal(err)
	}
	written, err := EnsureDesktop(data, "/another/copy/f4")
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Errorf("wrote %v, want the launcher and one icon", written)
	}
	if got := readFile(t, launcherPath(data)); got != DesktopEntry(live) {
		t.Errorf("the launcher must keep pointing at the existing executable:\n%s", got)
	}
	if readFile(t, stale) == "old" {
		t.Error("the stale icon was not replaced")
	}
}

func TestEnsureDesktopRepointsAManagedLauncherWhoseExecutableIsGone(t *testing.T) {
	data := t.TempDir()
	if _, err := InstallDesktop(data, "/gone/f4"); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureDesktop(data, "/new/place/f4"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, launcherPath(data)); got != DesktopEntry("/new/place/f4") {
		t.Errorf("launcher:\n%s", got)
	}
}

func TestEnsureDesktopAtStartRespectsOptOutSystemLauncherAndTempDir(t *testing.T) {
	if !SupportsDesktop() {
		t.Skip("no freedesktop launchers on this OS")
	}
	home := t.TempDir()
	system := t.TempDir()
	env := map[string]string{}
	oldHome, oldGetenv := UserHomeDir, Getenv
	UserHomeDir = func() (string, error) { return home, nil }
	Getenv = func(k string) string { return env[k] }
	t.Cleanup(func() { UserHomeDir, Getenv = oldHome, oldGetenv })
	// t.TempDir lives under the temporary directory, which the function must
	// refuse; point the temporary directory elsewhere for this test.
	scratch := filepath.Join(home, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", scratch)
	t.Setenv("TMP", scratch)
	t.Setenv("TEMP", scratch)
	exe := filepath.Join(home, "bin", "f4")
	launcher := filepath.Join(home, ".local", "share", "applications", DesktopFileName)

	env[OptOutEnv] = "1"
	if w, err := EnsureDesktopAtStart(exe); err != nil || len(w) != 0 {
		t.Errorf("opt-out: %v, %v", w, err)
	}
	delete(env, OptOutEnv)

	env["XDG_DATA_DIRS"] = system
	if err := os.MkdirAll(filepath.Join(system, "applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, "applications", DesktopFileName), []byte("x"), 0o644); err != nil { // #nosec G306 -- test file.
		t.Fatal(err)
	}
	if w, err := EnsureDesktopAtStart(exe); err != nil || len(w) != 0 {
		t.Errorf("system launcher: %v, %v", w, err)
	}
	env["XDG_DATA_DIRS"] = ""

	if w, err := EnsureDesktopAtStart(filepath.Join(os.TempDir(), "f4-build", "f4")); err != nil || len(w) != 0 {
		t.Errorf("temp dir: %v, %v", w, err)
	}
	if _, err := os.Stat(launcher); err == nil {
		t.Fatal("something was written although every guard said no")
	}

	if w, err := EnsureDesktopAtStart(exe); err != nil || len(w) == 0 {
		t.Fatalf("normal start: %v, %v", w, err)
	}
	if got := readFile(t, launcher); !strings.Contains(got, "TryExec="+exe+"\n") {
		t.Errorf("launcher:\n%s", got)
	}
}
