package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/vtui"
)

// Portable mode, Far3 style: a small <exe>.ini (or f4.ini shared by f4 and
// f4-gui) next to the binary decides where the profile lives. Everything the
// binary writes — settings, history, macros, plugins, crash logs — then stays
// in that profile, so the whole directory can be moved to another machine.
//
// The ini is deliberately read before anything else (see GetF4ConfigDir), so
// no setting stored *inside* the profile can switch the mode: the program
// would not know which profile to read it from. The Options → Portable mode
// dialog therefore edits <exe>.ini and asks for a restart instead of flipping
// a live flag.

// portableIniName is the file name shared by every binary in the directory
// (f4, f4-gui, f4.exe, f4-gui.exe), as asked for in unxed/f4#274.
const portableIniName = "f4.ini"

// portableProfileSubdirs are created inside a fresh portable profile so a user
// browsing the directory sees where macros, plugins and styles go instead of
// an empty folder. Each name matches what the corresponding loader reads.
var portableProfileSubdirs = []string{
	filepath.Join("Macros", "scripts"),
	"plugring",
	"settings",
	"styles",
}

// portableIniPath returns the ini GetF4ConfigDir reads for the given
// executable: <exe>.ini when it exists, otherwise f4.ini in the same
// directory. The second file need not exist; the caller decides whether a
// missing file matters.
func portableIniPath(exe string) string {
	own := exe + ".ini"
	// #nosec G703 -- own is the executable path plus ".ini", not an untrusted path component.
	if _, err := os.Stat(own); err == nil {
		return own
	}
	return filepath.Join(filepath.Dir(exe), portableIniName)
}

// expandProfileVars expands %NAME% (Far/Windows style) and $NAME / ${NAME}
// (Unix style) in a Profile= value. F4HOME always means exeDir, even when the
// process environment carries a different value, so a profile path in an ini
// that travels with the binary keeps pointing next to that binary.
func expandProfileVars(value, exeDir string) string {
	lookup := func(name string) string {
		if strings.EqualFold(name, "F4HOME") {
			return exeDir
		}
		return os.Getenv(name)
	}
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '%' {
			end := strings.IndexByte(value[i+1:], '%')
			if end > 0 {
				out.WriteString(lookup(value[i+1 : i+1+end]))
				i += end + 1
				continue
			}
		}
		out.WriteByte(value[i])
	}
	return os.Expand(out.String(), lookup)
}

// currentPortableIniPath is portableIniPath for the running binary.
func currentPortableIniPath() string {
	exe, err := osExecutable()
	if err != nil {
		exe = os.Args[0]
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return portableIniPath(exe)
}

// setPortableMode writes UseSystemProfiles into the ini next to the binary,
// creating the file when it does not exist. Any other key or comment in the
// file is kept, so a hand-written Profile= survives toggling the checkbox.
// The change is picked up on the next start; the running process keeps using
// the profile it opened.
func setPortableMode(iniPath string, enable bool) error {
	data, err := os.ReadFile(iniPath) // #nosec G304 -- iniPath is derived from the executable path.
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	value := "1"
	if enable {
		value = "0"
	}
	updated := updateIniValues(data, "General", map[string]string{"UseSystemProfiles": value})
	// Trim the blank line updateIniValues puts before a brand new section so
	// a freshly created file does not start with an empty line.
	updated = []byte(strings.TrimLeft(string(updated), "\r\n"))
	return os.WriteFile(iniPath, updated, 0600)
}

// ensureProfileLayout creates dir and the conventional subdirectories.
func ensureProfileLayout(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	for _, sub := range portableProfileSubdirs {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0700); err != nil {
			return err
		}
	}
	return nil
}

// copyProfileDir copies every file under src into dst, keeping files that
// already exist in dst and skipping crash logs. It never deletes anything, so
// switching modes twice cannot lose data: the user ends up with two copies
// rather than none.
func copyProfileDir(src, dst string) error {
	src, dst = filepath.Clean(src), filepath.Clean(dst)
	if src == dst {
		return nil
	}
	if strings.HasPrefix(dst, src+string(filepath.Separator)) {
		return fmt.Errorf("profile %q cannot be copied into itself", src)
	}
	info, err := os.Stat(src)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", src)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0700)
		}
		if d.IsDir() && rel == "crashes" {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if _, err := os.Lstat(target); err == nil {
			return nil
		}
		return copyFileNoClobber(path, target)
	})
}

func copyFileNoClobber(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src comes from walking the profile directory.
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- dst is inside the target profile.
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// systemProfileDir is the per-user directory f4 uses when it is not portable.
func systemProfileDir() string {
	sysDir, _ := userConfigDir()
	return filepath.Join(sysDir, "f4")
}

// portableProfileDir is the directory a portable f4 would use with the
// current <exe>.ini (honoring Profile= when present).
func portableProfileDir() string {
	iniPath := currentPortableIniPath()
	return portableProfileDirFor(filepath.Dir(iniPath), LoadIni(iniPath))
}

// actionPortableSettings is Options → Portable mode. It shows where the
// profile lives now, lets the user move it next to the program (or back to
// the user directory), optionally copies the current profile over, and tells
// them a restart is needed — the only honest answer given how the mode is
// detected (see the comment at the top of this file).
func actionPortableSettings(pf *PanelsFrame) {
	iniPath := currentPortableIniPath()
	wasPortable := IsPortableProfile()

	width, height := 70, 14
	dlg := vtui.NewCenteredDialog(width, height, Msg("PortableSettings.Title"))
	dlg.ShowClose = true

	chkPortable := vtui.NewCheckbox(0, 0, Msg("PortableSettings.Enable"), false)
	if wasPortable {
		chkPortable.State = 1
	}
	chkCopy := vtui.NewCheckbox(0, 0, Msg("PortableSettings.CopyProfile"), false)
	chkCopy.State = 1

	// Both paths can be long (a deep %APPDATA% or a build sandbox); keep the
	// tail, which is the part that tells the two locations apart.
	pathLine := func(key, path string) string {
		label := fmt.Sprintf(Msg(key), "")
		return label + truncPathLeft(path, width-4-vtui.StringWidth(label))
	}
	current := vtui.NewText(0, 0, pathLine("PortableSettings.Current", GetF4ConfigDir()), 0)
	iniInfo := vtui.NewText(0, 0, pathLine("PortableSettings.IniFile", iniPath), 0)
	note := vtui.NewText(0, 0, Msg("PortableSettings.Note"), 0)
	note2 := vtui.NewText(0, 0, Msg("PortableSettings.Note2"), 0)

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(current)
	dlg.AddItem(iniInfo)
	dlg.AddItem(chkPortable)
	dlg.AddItem(chkCopy)
	dlg.AddItem(note)
	dlg.AddItem(note2)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(current, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(iniInfo, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(chkPortable, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(chkCopy, vtui.Margins{Left: 3}, vtui.AlignLeft)
	vbox.Add(note, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(note2, vtui.Margins{}, vtui.AlignLeft)

	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		enable := chkPortable.State == 1
		if enable == wasPortable {
			dlg.Close()
			return
		}
		if err := applyPortableMode(iniPath, enable, chkCopy.State == 1); err != nil {
			vtui.ShowMessage(Msg("Error.Title"), err.Error(), []string{Msg("vtui.Ok")})
			return
		}
		dlg.Close()
		vtui.ShowMessage(Msg("PortableSettings.Title"), Msg("PortableSettings.Restart"), []string{Msg("vtui.Ok")})
	}

	vtui.FrameManager.Push(dlg)
}

// applyPortableMode flushes what the running instance has in memory, copies
// the profile if asked, and only then rewrites the ini. Ordering matters: if
// the copy fails the ini is untouched and the next start is unchanged.
func applyPortableMode(iniPath string, enable, copyProfile bool) error {
	SaveConfig()
	src := GetF4ConfigDir()
	var dst string
	if enable {
		dst = portableProfileDir()
		if err := ensureProfileLayout(dst); err != nil {
			return err
		}
	} else {
		dst = systemProfileDir()
		if err := os.MkdirAll(dst, 0700); err != nil {
			return err
		}
	}
	if copyProfile {
		if err := copyProfileDir(src, dst); err != nil {
			return err
		}
	}
	return setPortableMode(iniPath, enable)
}
