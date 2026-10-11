package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unxed/f4/internal/gui/assets/icon"
)

// Desktop integration (f4#1290). A single-file install has neither the
// .desktop entry nor the icons that the release archives carry, so the window
// of `f4 --gui` shows the window manager's default icon in the task bar. These
// files are written under the user's own XDG data directory: no sudo, and the
// desktop picks them up at once (or after the next login).

const (
	// DesktopFileName is the entry's name; on Wayland the window's application
	// ID (org.unxed.f4) is matched against it.
	DesktopFileName = "org.unxed.f4.desktop"
	// IconName is the Icon= value and the base name of every icon file.
	IconName = "io.github.unxed.f4"
	// ManagedKey marks a launcher that f4 wrote itself. EnsureDesktop keeps
	// such a file up to date, and never touches a launcher without it: that one
	// is the user's own (a different Exec=, a different backend) and stays.
	ManagedKey = "X-F4-Managed"
)

// SupportsDesktop reports whether this system uses freedesktop .desktop files.
func SupportsDesktop() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "ios", "android", "plan9", "js", "wasip1":
		return false
	}
	return true
}

// DataHome is the user's XDG data directory: $XDG_DATA_HOME when it is an
// absolute path, ~/.local/share otherwise.
func DataHome(home string, getenv func(string) string) string {
	if dir := getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(home, ".local", "share")
}

// execQuote writes path as one argument of an Exec= line, the way the
// Desktop Entry Specification asks: a name with reserved characters is put in
// double quotes, with ", `, $ and \ escaped, and the backslash written four
// times because the string escape is applied first.
func execQuote(path string) string {
	path = strings.ReplaceAll(path, "%", "%%")
	if !strings.ContainsAny(path, " \t\n\"'\\><~|&;$*?#()`") {
		return path
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range path {
		switch r {
		case '\\':
			b.WriteString(`\\\\`)
		case '"', '`', '$':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// DesktopEntry is the launcher text for the f4 executable at exePath. It is the
// entry packaging/linux/org.unxed.f4.desktop ships, with the executable's full
// path (the desktop's PATH need not contain ~/.local/bin).
func DesktopEntry(exePath string) string {
	return "[Desktop Entry]\n" +
		"Version=1.0\n" +
		"Type=Application\n" +
		"Name=f4\n" +
		"GenericName=File Manager\n" +
		"Comment=Far Manager / far2l-style file manager\n" +
		"Exec=" + execQuote(exePath) + " --gui\n" +
		"TryExec=" + exePath + "\n" +
		"Icon=" + IconName + "\n" +
		"StartupWMClass=org.unxed.f4\n" +
		ManagedKey + "=true\n" +
		"Terminal=false\n" +
		"Categories=System;FileManager;\n"
}

// writeFileAtomic writes data next to path and renames it into place, so a
// desktop reading the file never sees half of it.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".f4-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil { // #nosec G302 -- a launcher and icons are meant to be world-readable.
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// iconFile is one icon to put under the user's data directory.
type iconFile struct {
	path string
	data []byte
}

// iconFiles lists every icon file under dataHome, with its content.
func iconFiles(dataHome string) ([]iconFile, error) {
	svg, err := icon.Files.ReadFile("f4.svg")
	if err != nil {
		return nil, err
	}
	files := []iconFile{{filepath.Join(dataHome, "icons", "hicolor", "scalable", "apps", IconName+".svg"), svg}}
	for _, size := range icon.PNGSizes {
		png, err := icon.Files.ReadFile(fmt.Sprintf("generated/f4-%d.png", size))
		if err != nil {
			return nil, err
		}
		dir := fmt.Sprintf("%dx%d", size, size)
		files = append(files, iconFile{filepath.Join(dataHome, "icons", "hicolor", dir, "apps", IconName+".png"), png})
	}
	return files, nil
}

func launcherPath(dataHome string) string {
	return filepath.Join(dataHome, "applications", DesktopFileName)
}

// InstallDesktop writes the launcher for the executable at exePath and the
// application icons under dataHome, and returns the files written. Running it
// again rewrites the same files.
func InstallDesktop(dataHome, exePath string) ([]string, error) {
	icons, err := iconFiles(dataHome)
	if err != nil {
		return nil, err
	}
	files := append([]iconFile{{launcherPath(dataHome), []byte(DesktopEntry(exePath))}}, icons...)
	var written []string
	for _, f := range files {
		if err := writeFileAtomic(f.path, f.data); err != nil {
			return written, err
		}
		written = append(written, f.path)
	}
	return written, nil
}

// execOfEntry is the TryExec= path of a launcher text, or "".
func execOfEntry(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(line, "TryExec="); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// EnsureDesktop makes sure the launcher and icons are in place under
// dataHome, writing only what is missing or out of date, and returns the files
// it wrote. It is what a GUI start runs, so that an f4 that was installed by
// hand and updates itself gets its task bar icon without the user doing
// anything (f4#1290):
//
//   - a missing launcher is written for exePath;
//   - a launcher that f4 wrote earlier (ManagedKey) is rewritten when the
//     program's text moved on, keeping the executable it already points at
//     while that still exists, so two copies of f4 do not fight over it;
//   - a launcher without ManagedKey is the user's and is left alone;
//   - an icon file that differs from the embedded one is replaced.
func EnsureDesktop(dataHome, exePath string) ([]string, error) {
	var files []iconFile
	current, err := os.ReadFile(launcherPath(dataHome)) // #nosec G304 -- the user's own data directory.
	switch {
	case err != nil && !os.IsNotExist(err):
		return nil, err
	case err != nil:
		files = append(files, iconFile{launcherPath(dataHome), []byte(DesktopEntry(exePath))})
	case strings.Contains(string(current), "\n"+ManagedKey+"="):
		target := execOfEntry(string(current))
		// target comes from the user's own launcher and is only stat'ed.
		if info, statErr := os.Stat(target); target == "" || statErr != nil || info.IsDir() { // #nosec G703 -- existence check only.
			target = exePath
		}
		if want := DesktopEntry(target); string(current) != want {
			files = append(files, iconFile{launcherPath(dataHome), []byte(want)})
		}
	}
	icons, err := iconFiles(dataHome)
	if err != nil {
		return nil, err
	}
	for _, f := range icons {
		if have, err := os.ReadFile(f.path); err != nil || !bytes.Equal(have, f.data) { // #nosec G304 -- the user's own data directory.
			files = append(files, f)
		}
	}
	var written []string
	for _, f := range files {
		if err := writeFileAtomic(f.path, f.data); err != nil {
			return written, err
		}
		written = append(written, f.path)
	}
	return written, nil
}

// OptOutEnv is the environment variable that switches EnsureDesktopAtStart off.
const OptOutEnv = "F4_NO_DESKTOP_INSTALL"

// systemLauncherExists reports whether a package already put the launcher
// into one of the system data directories ($XDG_DATA_DIRS).
func systemLauncherExists(getenv func(string) string) bool {
	dirs := getenv("XDG_DATA_DIRS")
	if dirs == "" {
		dirs = "/usr/local/share:/usr/share"
	}
	for _, dir := range filepath.SplitList(dirs) {
		if filepath.IsAbs(dir) {
			if _, err := os.Stat(launcherPath(dir)); err == nil {
				return true
			}
		}
	}
	return false
}

// EnsureDesktopAtStart is called before a GUI window opens. It does nothing
// on systems without .desktop files, when OptOutEnv is set, when a package
// provides the launcher system-wide, or when f4 runs from the temporary
// directory (a throwaway build must not become the launcher). Any failure is
// returned for the debug log; it never stops f4 from starting.
func EnsureDesktopAtStart(exePath string) ([]string, error) {
	if !SupportsDesktop() || Getenv(OptOutEnv) != "" || systemLauncherExists(Getenv) {
		return nil, nil
	}
	home, err := UserHomeDir()
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	if abs, err := filepath.Abs(exePath); err == nil {
		exePath = abs
	}
	if tmp, err := filepath.EvalSymlinks(os.TempDir()); err == nil && strings.HasPrefix(exePath, tmp+string(filepath.Separator)) {
		return nil, nil
	}
	return EnsureDesktop(DataHome(home, Getenv), exePath)
}

// RunDesktopCLI serves `f4 --install-desktop`, and the desktop step of
// `f4 --install`: it makes the running f4 known to the desktop (launcher entry
// and icons in the user's XDG data directory), so the task bar shows f4's own
// icon. exePath is the executable the launcher starts. Returns the exit code.
func RunDesktopCLI(exePath string) int {
	if !SupportsDesktop() {
		fmt.Println("f4: desktop integration (a .desktop launcher and icons) is for Linux and BSD desktops; nothing to do here.")
		return 1
	}
	home, err := UserHomeDir()
	if err != nil {
		fmt.Printf("f4: could not determine your home directory: %v\n", err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	if abs, err := filepath.Abs(exePath); err == nil {
		exePath = abs
	}
	dataHome := DataHome(home, Getenv)
	if _, err := InstallDesktop(dataHome, exePath); err != nil {
		fmt.Printf("f4: could not install the launcher and icons under %s: %v\n", dataHome, err)
		return 1
	}
	fmt.Printf("Installed the launcher %s and the icons under %s.\n",
		filepath.Join(dataHome, "applications", DesktopFileName), filepath.Join(dataHome, "icons", "hicolor"))
	fmt.Println("If the task bar still shows a default icon, close f4 and start it again (or log out and in once).")
	return 0
}
