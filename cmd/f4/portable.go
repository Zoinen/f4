package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/vtinput"
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

// copyProfileDir copies every non-crash file under src into dst, keeping files
// that already exist in dst. It never deletes anything, so switching modes
// twice cannot lose data: the user ends up with two copies rather than none.
func copyProfileDir(src, dst string) error {
	return transferProfileDir(src, dst, true)
}

// transferProfileDir copies files under src into dst. When skipCrashes is
// false, crash logs are included. It keeps existing destination files so that
// Copy is non-destructive.
func transferProfileDir(src, dst string, skipCrashes bool) error {
	return transferProfileDirWithPolicy(src, dst, skipCrashes, false)
}

// moveProfileDir transfers every source file and rejects destination conflicts.
// The caller can therefore remove src only after this function succeeds without
// risking data loss from a non-overwriting copy.
func moveProfileDir(src, dst string) error {
	return transferProfileDirWithPolicy(src, dst, false, true)
}

func transferProfileDirWithPolicy(src, dst string, skipCrashes, failOnConflict bool) error {
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
	if failOnConflict {
		if err := rejectTransferConflicts(src, dst, skipCrashes); err != nil {
			return err
		}
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
		if skipCrashes && d.IsDir() && rel == "crashes" {
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
			if failOnConflict {
				return fmt.Errorf("cannot move profile: destination already contains %q", rel)
			}
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return copyFileNoClobber(path, target)
	})
}

func rejectTransferConflicts(src, dst string, skipCrashes bool) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			if info, err := os.Lstat(dst); err == nil && !info.IsDir() {
				return fmt.Errorf("cannot move profile: destination %q is not a directory", dst)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		if skipCrashes && d.IsDir() && rel == "crashes" {
			return filepath.SkipDir
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if info, err := os.Lstat(target); err == nil && !info.IsDir() {
				return fmt.Errorf("cannot move profile: destination %q is not a directory", rel)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("cannot move profile: destination already contains %q", rel)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	})
}

func copyFileNoClobber(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src comes from walking the profile directory.
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- dst is inside the target profile.
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
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

// portableSettingsDialog keeps the dialog height fixed while forwarding the
// resize gesture as a horizontal-only resize. vtui windows expose the resize
// corner for every window, and this small wrapper keeps this compact dialog's
// vertical geometry stable without changing the behavior of other windows.
type portableSettingsDialog struct {
	*vtui.Window
	fixedHeight      int
	resizing         bool
	resizeStartX2    int
	resizeStartWidth int
}

// ResizeConsole keeps a width chosen by the user when the terminal or main
// window is resized. vtui.Window.ResizeConsole uses the content width from
// dialog creation, which would discard a later horizontal resize.
func (d *portableSettingsDialog) ResizeConsole(screenW, screenH int) {
	width := d.X2 - d.X1 + 1
	height := d.Y2 - d.Y1 + 1
	if width <= screenW && height <= screenH {
		d.Center(screenW, screenH)
		return
	}
	d.Window.ResizeConsole(screenW, screenH)
}

func (d *portableSettingsDialog) ProcessMouse(e *vtinput.InputEvent) bool {
	if d.resizing {
		if e.ButtonState == 0 {
			d.resizing = false
			return true
		}
		delta := int(e.MouseX) - d.resizeStartX2
		width := d.resizeStartWidth + 2*delta
		if width < d.MinW {
			width = d.MinW
			delta = (width - d.resizeStartWidth) / 2
		}
		d.ChangeSize(width, d.fixedHeight)
		startX1 := d.resizeStartX2 - d.resizeStartWidth + 1
		d.MoveRelative(startX1-delta-d.X1, 0)
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown &&
		int(e.MouseX) == d.X2 && int(e.MouseY) == d.Y2 {
		d.resizing = true
		d.resizeStartX2 = d.X2
		d.resizeStartWidth = d.X2 - d.X1 + 1
		return true
	}

	return d.Window.ProcessMouse(e)
}

// portableSettingsPathText recalculates the visible tail of a profile path
// whenever AutoLayout gives the line a new width.
type portableSettingsPathText struct {
	*vtui.Text
	key  string
	path string
}

func newPortableSettingsPathText(key, path string, width int) *portableSettingsPathText {
	t := &portableSettingsPathText{
		Text: vtui.NewText(0, 0, "", 0),
		key:  key,
		path: path,
	}
	t.SetPosition(0, 0, width-1, 0)
	return t
}

func (t *portableSettingsPathText) SetPosition(x1, y1, x2, y2 int) {
	t.Text.SetPosition(x1, y1, x2, y2)
	label := fmt.Sprintf(Msg(t.key), "")
	available := x2 - x1 + 1 - vtui.StringWidth(label)
	if available < 0 {
		available = 0
	}
	t.SetText(label + truncPathLeft(t.path, available))
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
	dlg := &portableSettingsDialog{
		Window:      vtui.NewCenteredDialog(width, height, Msg("PortableSettings.Title")),
		fixedHeight: height,
	}
	dlg.ShowClose = true
	dlg.SetHelp("PortableSettings")

	chkPortable := vtui.NewCheckbox(0, 0, Msg("PortableSettings.Enable"), false)
	if wasPortable {
		chkPortable.State = 1
	}
	transferModes := []string{Msg("PortableSettings.Copy"), Msg("PortableSettings.Move")}
	comboTransfer := vtui.NewComboBox(0, 0, 18, transferModes)
	comboTransfer.DropdownOnly = true
	comboTransfer.Menu.SetSelectPos(0)
	comboTransfer.Edit.SetText(transferModes[0])
	lblTransfer := vtui.NewLabel(0, 0, Msg("PortableSettings.Transfer"), comboTransfer)

	current := newPortableSettingsPathText("PortableSettings.Current", GetF4ConfigDir(), width-4)
	iniInfo := newPortableSettingsPathText("PortableSettings.IniFile", iniPath, width-4)
	note := vtui.NewText(0, 0, Msg("PortableSettings.Note"), 0)
	note2 := vtui.NewText(0, 0, Msg("PortableSettings.Note2"), 0)

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(current)
	dlg.AddItem(iniInfo)
	dlg.AddItem(chkPortable)
	dlg.AddItem(lblTransfer)
	dlg.AddItem(comboTransfer)
	dlg.AddItem(note)
	dlg.AddItem(note2)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	transferRow := vtui.NewHBoxLayout(0, 0, width-4, 1)
	transferRow.Add(lblTransfer, vtui.Margins{Left: 3, Right: 1}, vtui.AlignLeft)
	transferRow.Add(comboTransfer, vtui.Margins{}, vtui.AlignLeft)

	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

	layout := vtui.NewAutoLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	layout.SetGrowMode(vtui.GrowHiX)
	layout.PinTop(current, 0)
	layout.FillWidth(current, 0, 0)
	layout.FillWidth(iniInfo, 0, 0)
	layout.PinLeft(chkPortable, 0)
	layout.FillWidth(transferRow, 0, 0)
	layout.FillWidth(note, 0, 0)
	layout.FillWidth(note2, 0, 0)
	layout.FillWidth(buttons, 0, 0)
	layout.StackVertical(0, current, iniInfo)
	layout.StackVertical(1, iniInfo, chkPortable)
	layout.StackVertical(0, chkPortable, transferRow)
	layout.StackVertical(1, transferRow, note)
	layout.StackVertical(0, note, note2)
	layout.StackVertical(1, note2, buttons)
	layout.Apply()
	dlg.AddItem(layout)

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		enable := chkPortable.State == 1
		if enable == wasPortable {
			dlg.Close()
			return
		}
		if err := applyPortableMode(iniPath, enable, comboTransfer.Menu.SelectPos == 1); err != nil {
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
func applyPortableMode(iniPath string, enable, moveProfile bool) error {
	SaveConfig()
	src := GetF4ConfigDir()
	var dst string
	if enable {
		dst = portableProfileDir()
	} else {
		dst = systemProfileDir()
	}
	if moveProfile {
		if err := moveProfileDir(src, dst); err != nil {
			return err
		}
	} else {
		if err := copyProfileDir(src, dst); err != nil {
			return err
		}
	}
	// Create the conventional layout only after the transfer has succeeded.
	// In particular, a destination accidentally nested inside src must not
	// modify the source before transferProfileDir can reject it.
	if enable {
		if err := ensureProfileLayout(dst); err != nil {
			return err
		}
	} else if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}
	if err := setPortableMode(iniPath, enable); err != nil {
		return err
	}
	if moveProfile && filepath.Clean(src) != filepath.Clean(dst) {
		if err := os.RemoveAll(src); err != nil {
			return err
		}
	}
	return nil
}
