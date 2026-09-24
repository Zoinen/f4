package dialog

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// AboutFacts is what f4:about needs from above this package: the version and
// elevation helpers, the shell, the clipboard and the plugin host live in
// packages dialog may not import, so the composition root gathers them.
type AboutFacts struct {
	Version         string
	Admin           string // "" when f4 is not elevated
	Shell           string
	Plugins         []string
	CommandPrefixes []AboutCommandPrefix
	// Copy puts the whole list on the clipboard (Ctrl+C, Ctrl+Ins).
	Copy func(string)
}

// AboutCommandPrefix is one registered command-line prefix and who owns it.
type AboutCommandPrefix struct {
	Prefix string
	Owner  string
}

// aboutRow is one line of f4:about. A divider separates two groups of rows.
type aboutRow struct {
	label   string
	value   string
	divider bool
}

// aboutLine is a row laid out for display or copying.
type aboutLine struct {
	text    string
	divider bool
}

// aboutEnvVars are the variables far2l's about list reports, plus the ones f4
// itself reads: where it lives, nesting, vtui's debug and graphics switches,
// the Windows Terminal session and the locale codepages are guessed from.
var aboutEnvVars = []string{
	"HOSTNAME", "USER", "USERNAME", "HOME",
	"F4HOME", "F4_NESTED", "VTUI_DEBUG", "VTUI_GRAPHICS",
	"SHELL", "COMSPEC",
	"TERM", "COLORTERM", "TERM_PROGRAM", "WT_SESSION",
	"LANG", "LC_ALL", "LC_CTYPE",
	"XDG_SESSION_TYPE", "XDG_SESSION_DESKTOP", "XDG_CURRENT_DESKTOP",
	"GDK_BACKEND", "DESKTOP_SESSION",
	"WSL_DISTRO_NAME", "WSL2_GUI_APPS_ENABLED",
	"DISPLAY", "WAYLAND_DISPLAY",
	"GTK_IM_MODULE", "QT_IM_MODULE", "XMODIFIERS",
}

// aboutHideEmpty is far2l's static b_hide_empty: rows without a value start
// hidden, and Ctrl+H flips that for the rest of the session.
var aboutHideEmpty = true

// ShowAbout opens f4:about, modelled on far2l's FarAbout: a read-only list of
// what a bug report needs to say about this f4 and the system it runs on.
//
// The row labels are English on purpose, as they are in far2l: the list is
// copied into bug reports, and it has to read the same whatever language the
// interface is in.
func ShowAbout(facts AboutFacts) {
	showAbout(facts, 0)
}

func showAbout(facts AboutFacts, selected int) {
	if vtui.FrameManager == nil {
		return
	}
	rows := collectAboutRows(facts)
	copyText := aboutText(rows)
	hint := i18n.Msg("About.Hint")
	menu := vtui.NewVMenu(i18n.Msg("About.Title"))
	menu.SetBottomTitle(hint)
	// The list is read, not chosen from: a click only moves the selection.
	menu.IgnoreSingleClick = true

	fill := func(selected int) {
		lines := aboutLines(rows, aboutHideEmpty)
		scrW := vtui.FrameManager.GetScreenSize()
		scrH := vtui.FrameManager.GetScreenHeight()

		// Two border columns, the leading space and a column for the
		// scrollbar around the text.
		const chrome = 5
		menu.SetTitle(aboutTitle(aboutHideEmpty))
		w := vtui.StringWidth(menu.GetTitle()) + chrome + 1
		if hw := vtui.StringWidth(hint) + chrome + 1; hw > w {
			w = hw
		}
		for _, line := range lines {
			if lw := vtui.StringWidth(line.text) + chrome; lw > w {
				w = lw
			}
		}
		if maxW := scrW - 4; maxW >= 20 && w > maxW {
			w = maxW
		} else if w > scrW {
			w = scrW
		}
		h := len(lines) + 2
		if maxH := scrH - 4; maxH >= 3 && h > maxH {
			h = maxH
		} else if h > scrH {
			h = scrH
		}

		menu.Items = aboutMenuItems(lines, w-chrome)
		menu.ItemCount = len(menu.Items)
		x, y := (scrW-w)/2, (scrH-h)/2
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		menu.SetPosition(x, y, x+w-1, y+h-1)
		menu.SetSelectPos(min(max(selected, 0), max(len(menu.Items)-1, 0)))
	}
	fill(selected)

	// A double click confirms, and a confirmed menu closes. far2l keeps
	// far:about open, so the list comes straight back on the same row.
	menu.OnAction = func(pos int) {
		showAbout(facts, pos)
	}

	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		shift := e.ControlKeyState&vtinput.ShiftPressed != 0
		plainCtrl := ctrl && !alt && !shift
		switch {
		case e.VirtualKeyCode == vtinput.VK_RETURN:
			// There is nothing to choose; far2l keeps the list open too.
			return true
		case plainCtrl && (e.VirtualKeyCode == vtinput.VK_C || e.VirtualKeyCode == vtinput.VK_INSERT || e.VirtualKeyCode == vtinput.VK_NUMPAD0):
			if facts.Copy != nil {
				facts.Copy(copyText)
				toast.Show(i18n.Msg("About.Copied"), 2*time.Second)
			}
			return true
		case plainCtrl && e.VirtualKeyCode == vtinput.VK_H:
			aboutHideEmpty = !aboutHideEmpty
			fill(0)
			vtui.FrameManager.Redraw()
			return true
		}
		return false
	}
	vtui.FrameManager.Push(menu)
}

// aboutTitle is the list title, marked the way far2l marks far:about while
// rows without a value are hidden.
func aboutTitle(hideEmpty bool) string {
	title := i18n.Msg("About.Title")
	if hideEmpty {
		title += " *"
	}
	return title
}

// collectAboutRows gathers the list in far2l's order: the build, the running
// process, the directories, the operating system, the environment, and what
// is plugged in.
func collectAboutRows(facts AboutFacts) []aboutRow {
	// The profile directory is resolved first: resolving it is what exports
	// F4HOME when the environment did not already carry it.
	profile := config.GetF4ConfigDir()
	if config.IsPortableProfile() {
		profile = aboutQuoted(profile) + " (portable)"
	} else {
		profile = aboutQuoted(profile)
	}
	admin := facts.Admin
	if admin == "" {
		admin = "-"
	}

	rows := []aboutRow{
		{label: "f4 version", value: facts.Version},
		{label: "Go toolchain", value: runtime.Version() + " (" + runtime.Compiler + ")"},
		{label: "Build settings", value: aboutBuildSettings()},
		{label: "Platform", value: runtime.GOOS + "/" + runtime.GOARCH},
		{label: "Backend", value: aboutBackend()},
	}
	// What the backend said about itself: far2l's "System component" lines.
	for _, detail := range vtui.BackendDetails() {
		rows = append(rows, aboutRow{label: "Backend detail", value: detail})
	}
	rows = append(rows, []aboutRow{
		{label: "Screen size in cells", value: fmt.Sprintf("%dx%d", vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())},
		{label: "Admin", value: admin},
		{label: "PID", value: strconv.Itoa(os.Getpid())},
		{label: "Main | Help languages", value: config.App.Language + " | " + config.App.HelpLanguage},
		{label: "ANSI | OEM codepages", value: fmt.Sprintf("%d | %d", vfs.SystemANSICodepage(), vfs.SystemOEMCodepage())},
		{label: "f4 directory (F4HOME)", value: aboutQuoted(os.Getenv("F4HOME"))},
		{label: "Profile directory", value: profile},
		{label: "Settings file", value: aboutQuoted(config.GetUserConfigIniPath())},
		{label: "Temp directory", value: aboutQuoted(os.TempDir())},
		{label: "Command shell", value: aboutQuoted(facts.Shell)},
		{divider: true},
	}...)
	rows = append(rows, aboutOSRows()...)
	for _, name := range aboutEnvVars {
		rows = append(rows, aboutRow{label: name, value: os.Getenv(name)})
	}

	rows = append(rows, aboutRow{divider: true})
	for _, prefix := range facts.CommandPrefixes {
		rows = append(rows, aboutRow{label: "Command prefix", value: prefix.Prefix + ": (" + prefix.Owner + ")"})
	}
	rows = append(rows, aboutRow{label: "Number of plugins", value: strconv.Itoa(len(facts.Plugins))})
	for i, name := range facts.Plugins {
		rows = append(rows, aboutRow{label: fmt.Sprintf("Plugin #%02d", i+1), value: name})
	}
	return rows
}

// aboutLines lays rows out with every label right-aligned to the longest
// one, as far2l does. With hideEmpty, rows without a value are left out, and
// a divider that would follow another divider, or open or close the list, is
// dropped so that a group hidden entirely leaves no trace.
func aboutLines(rows []aboutRow, hideEmpty bool) []aboutLine {
	width := 0
	for _, row := range rows {
		if !row.divider && len(row.label) > width {
			width = len(row.label)
		}
	}
	var lines []aboutLine
	for _, row := range rows {
		if row.divider {
			if len(lines) > 0 && !lines[len(lines)-1].divider {
				lines = append(lines, aboutLine{divider: true})
			}
			continue
		}
		if hideEmpty && row.value == "" {
			continue
		}
		text := strings.TrimRight(fmt.Sprintf("%*s: %s", width, row.label, row.value), " ")
		lines = append(lines, aboutLine{text: text})
	}
	if n := len(lines); n > 0 && lines[n-1].divider {
		lines = lines[:n-1]
	}
	return lines
}

// aboutText is what Ctrl+C copies: every row, hidden or not, the way far2l
// copies its whole list. A divider becomes an empty line.
func aboutText(rows []aboutRow) string {
	lines := aboutLines(rows, false)
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = line.text
	}
	return strings.Join(out, "\n")
}

// aboutMenuItems turns lines into menu rows no wider than textWidth. A value
// may hold an ampersand, which a menu would take for a hotkey marker.
func aboutMenuItems(lines []aboutLine, textWidth int) []vtui.MenuItem {
	items := make([]vtui.MenuItem, 0, len(lines))
	for _, line := range lines {
		if line.divider {
			items = append(items, vtui.MenuItem{Separator: true})
			continue
		}
		text := vtui.TruncateMiddle(line.text, textWidth)
		items = append(items, vtui.MenuItem{Text: strings.ReplaceAll(text, "&", "&&")})
	}
	return items
}

// aboutBuildSettings reports the build flags that change what the binary can
// do: cgo, the build tags and trimpath.
func aboutBuildSettings() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var parts []string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "CGO_ENABLED", "-tags", "-trimpath":
			parts = append(parts, setting.Key+"="+setting.Value)
		}
	}
	return strings.Join(parts, " ")
}

// aboutBackend names the renderer, with the backend a graphical host claimed.
func aboutBackend() string {
	name := vtui.FrameManager.GetBackendName()
	if active := vtui.ActiveBackend(); active != "" {
		name += " [" + active + "]"
	}
	return name
}

// aboutQuoted quotes a path the way far2l's list does, so that leading and
// trailing spaces show. An empty value stays empty, which hides the row.
func aboutQuoted(value string) string {
	if value == "" {
		return ""
	}
	return `"` + value + `"`
}

// parseOSReleasePrettyName reads PRETTY_NAME from an os-release file.
func parseOSReleasePrettyName(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		value, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "PRETTY_NAME=")
		if !ok {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
		return strings.Trim(value, `"'`)
	}
	return ""
}
