package main

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// AltPanel is a Panel that mirrors information about another (source)
// FileSystemPanel's current selection — Info, Quick view, Tree, etc.
// It sits in the passive slot alongside the source file panel and is
// swapped in/out by Ctrl+L / Ctrl+Q / Ctrl+T. Focus never sits on an
// AltPanel; it stays on the source (like far2l's info/qview/tree do).
type AltPanel interface {
	Panel
	// Source returns the file panel this alt panel is mirroring.
	Source() *FileSystemPanel
	// Kind identifies the alt-panel variant ("info", "quick_view",
	// "tree"), used by the toggle logic and future persistence.
	Kind() string
}

// InfoPanel is far2l's Ctrl+L information panel. It shows the
// active file panel's location and system context — computer/user,
// current directory and disk space. Per-file details ("Quick view")
// belong to Ctrl+Q (a follow-up PR); git status and description-file
// (README/Descript.ion) rendering are also deliberately deferred.
type InfoPanel struct {
	vtui.ScreenObject
	src     *FileSystemPanel
	frame   *vtui.BorderedFrame
	focused bool
}

// NewInfoPanel creates an info panel positioned over src's slot.
// The caller is expected to reposition it via SetPosition to fit the
// current layout (PanelsFrame.ResizeConsole does this).
func NewInfoPanel(src *FileSystemPanel) *InfoPanel {
	x1, y1, x2, y2 := src.GetPosition()
	ip := &InfoPanel{src: src}
	ip.SetVisible(true)
	ip.frame = vtui.NewBorderedFrame(x1, y1, x2, y2, vtui.SingleBox, Msg("InfoPanel.Title"))
	ip.frame.ColorBoxIdx = ColPanelBox
	ip.frame.ColorTitleIdx = ColPanelTitle
	// Fill the interior with the same attribute we render text in, so
	// character cells and the empty space around them share one bg —
	// no highlight strip behind text lines.
	ip.frame.ColorBackgroundIdx = ColPanelInfoText
	ip.SetPosition(x1, y1, x2, y2)
	return ip
}

func (ip *InfoPanel) SetPosition(x1, y1, x2, y2 int) {
	ip.ScreenObject.SetPosition(x1, y1, x2, y2)
	if ip.frame != nil {
		ip.frame.SetPosition(x1, y1, x2, y2)
	}
}

func (ip *InfoPanel) Source() *FileSystemPanel { return ip.src }
func (ip *InfoPanel) Kind() string             { return "info" }

// SetFocus flips the visible focus marker (title recolours). The alt
// panel doesn't consume keyboard input — commands still target the
// source file panel — but showing the frame as focused matches how
// far2l's Info/Quick view highlight themselves on Tab.
func (ip *InfoPanel) SetFocus(f bool) {
	ip.focused = f
	if ip.frame != nil {
		if f {
			ip.frame.ColorTitleIdx = ColPanelSelectedTitle
		} else {
			ip.frame.ColorTitleIdx = ColPanelTitle
		}
	}
}

func (ip *InfoPanel) IsFocused() bool { return ip.focused }

// ProcessKey / ProcessMouse do nothing — alt panels are display-only.
// Anything typed on the alt-panel slot falls through to the global
// handler, which will dispatch to the source file panel underneath.
func (ip *InfoPanel) ProcessKey(*vtinput.InputEvent) bool   { return false }
func (ip *InfoPanel) ProcessMouse(*vtinput.InputEvent) bool { return false }

// GetSelectedName proxies to the source so callers that inspect
// "the passive panel's selection" (drive menu, etc.) keep working.
func (ip *InfoPanel) GetSelectedName() string {
	if ip.src == nil {
		return ""
	}
	return ip.src.GetSelectedName()
}

func (ip *InfoPanel) Show(scr *vtui.ScreenBuf) {
	if ip.frame != nil {
		ip.frame.Show(scr)
	}
	// Bottom-border hint reminding the user of the units toggle,
	// same pattern the bookmarks dialog uses. Drawn on the ┴ line;
	// keeps the panel self-documenting so we don't need a menu entry.
	if ip.frame != nil && ip.Y2 > ip.Y1+1 {
		hint := Msg("InfoPanel.UnitsHint")
		if runewidth.StringWidth(hint) < ip.X2-ip.X1-1 {
			attrBox := vtui.Palette[ColPanelBox]
			scr.Write(ip.X1+2, ip.Y2, vtui.StringToCharInfo(hint, attrBox))
		}
	}
	innerW := ip.X2 - ip.X1 - 1 // room between the two vertical borders
	if innerW < 1 {
		return
	}
	attr := vtui.Palette[ColPanelInfoText]
	y := ip.Y1 + 1
	maxY := ip.Y2 - 1

	// Two-column row: label on the left, value right-aligned. Both use
	// the same attr as the frame background so the row reads as a
	// single flat block, matching far2l's InfoList layout.
	row := func(label, value string) {
		if y > maxY {
			return
		}
		labelPad := " " + label
		if value == "" {
			ip.writeLine(scr, labelPad, attr, innerW, y)
			y++
			return
		}
		labelW := runewidth.StringWidth(labelPad)
		valueW := runewidth.StringWidth(value)
		// Available room for spaces between label and value.
		space := innerW - labelW - valueW
		if space < 1 {
			// Truncate value to fit.
			roomForValue := innerW - labelW - 1
			if roomForValue < 1 {
				ip.writeLine(scr, labelPad, attr, innerW, y)
				y++
				return
			}
			value = runewidth.Truncate(value, roomForValue, "…")
			space = 1
		}
		text := labelPad + strings.Repeat(" ", space) + value
		ip.writeLine(scr, text, attr, innerW, y)
		y++
	}
	blank := func() {
		if y > maxY {
			return
		}
		ip.writeLine(scr, "", attr, innerW, y)
		y++
	}

	// wrapRow is like row but, for values too long to share a line
	// with their label, breaks the value on `sep` and continues it
	// on hanging continuation lines. Used for Flags where the value
	// is a naturally-splittable comma-list — Windows NTFS attributes
	// alone go past 60 cols. Falls back to plain row for short values.
	wrapRow := func(label, value, sep string) {
		labelPad := " " + label
		labelW := runewidth.StringWidth(labelPad)
		fitsInline := labelW+1+runewidth.StringWidth(value) <= innerW
		if fitsInline || value == "" {
			row(label, value)
			return
		}
		// Continuation lines hang two cells past the label start so
		// the wrapped value visually attaches to its label.
		hangStart := 3
		if hangStart >= innerW {
			hangStart = 1
		}
		hangIndent := strings.Repeat(" ", hangStart)
		hangRoom := innerW - hangStart
		if hangRoom < 1 {
			row(label, value)
			return
		}
		parts := strings.Split(value, sep)
		// First line: label followed by as many parts as fit right of it.
		gap := 1
		firstRoom := innerW - labelW - gap
		var first []string
		i := 0
		for i < len(parts) {
			piece := parts[i]
			if len(first) > 0 {
				piece = sep + piece
			}
			if runewidth.StringWidth(strings.Join(first, "")+piece) > firstRoom {
				break
			}
			if len(first) == 0 {
				first = append(first, parts[i])
			} else {
				first = append(first, sep+parts[i])
			}
			i++
		}
		firstValue := strings.Join(first, "")
		if firstValue == "" {
			// Value's first token alone doesn't fit next to the label;
			// put the label on its own line and wrap the value below.
			ip.writeLine(scr, labelPad, attr, innerW, y)
			y++
		} else {
			text := labelPad + strings.Repeat(" ", innerW-labelW-runewidth.StringWidth(firstValue)) + firstValue
			ip.writeLine(scr, text, attr, innerW, y)
			y++
		}
		// Continuation lines: greedy-pack the remaining parts.
		cur := ""
		flush := func() {
			if cur == "" || y > maxY {
				return
			}
			ip.writeLine(scr, hangIndent+cur, attr, innerW, y)
			y++
			cur = ""
		}
		for ; i < len(parts); i++ {
			piece := parts[i]
			if cur != "" {
				piece = sep + piece
			}
			if runewidth.StringWidth(cur+piece) > hangRoom {
				flush()
				piece = parts[i]
			}
			cur += piece
		}
		flush()
	}

	sectionHeader := func(text string) {
		if y > maxY {
			return
		}
		// Centered heading like far2l's DrawTitle inside InfoList.
		text = " " + text + " "
		w := runewidth.StringWidth(text)
		pad := 0
		if w < innerW {
			pad = (innerW - w) / 2
		}
		line := strings.Repeat("─", pad) + text + strings.Repeat("─", innerW-pad-w)
		if w > innerW {
			line = runewidth.Truncate(text, innerW, "…")
		}
		ip.writeLine(scr, line, attr, innerW, y)
		y++
	}

	// Header — computer / user.
	hostname, _ := os.Hostname()
	username := ""
	if u, err := user.Current(); err == nil {
		username = shortUsername(u.Username)
	}
	row(Msg("InfoPanel.Computer"), hostname)
	row(Msg("InfoPanel.User"), username)
	blank()

	// Filesystem.
	path := ""
	if ip.src != nil && ip.src.vfs != nil {
		path = ip.src.vfs.GetPath()
	}
	fsTitle := Msg("InfoPanel.FilesystemTitle")
	if fs, ok := fsInfo(path); ok {
		if fs.Type != "" {
			fsTitle = fmt.Sprintf("%s (%s)", fsTitle, fs.Type)
		}
		sectionHeader(fsTitle)
		row(Msg("InfoPanel.Total"), formatBytes(fs.Total))
		row(Msg("InfoPanel.Free"), formatBytes(fs.Free))
		if fs.Label != "" {
			row(Msg("InfoPanel.Label"), fs.Label)
		}
		if fs.Serial != "" {
			row(Msg("InfoPanel.Serial"), fs.Serial)
		}
		row(Msg("InfoPanel.CurrentDir"), path)
		if fs.Mount != "" && fs.Mount != path {
			row(Msg("InfoPanel.Mount"), fs.Mount)
		}
		if fs.MaxFilename > 0 {
			row(Msg("InfoPanel.MaxFilename"), fmt.Sprintf("%d", fs.MaxFilename))
		}
		if fs.Flags != "" {
			wrapRow(Msg("InfoPanel.Flags"), fs.Flags, ",")
		}
	} else {
		sectionHeader(fsTitle)
		row(Msg("InfoPanel.CurrentDir"), path)
	}

	// Memory. Same numbers as far2l's InfoList reads via sysinfo(2)
	// on Linux — see mem_info_unix.go for the exact formula.
	if mem, ok := memInfo(); ok {
		blank()
		sectionHeader(Msg("InfoPanel.MemoryTitle"))
		row(Msg("InfoPanel.MemLoad"), fmt.Sprintf("%d%%", mem.LoadPercent))
		row(Msg("InfoPanel.MemTotal"), formatBytes(mem.Total))
		row(Msg("InfoPanel.MemFree"), formatBytes(mem.Free))
		if mem.Shared > 0 {
			row(Msg("InfoPanel.MemShared"), formatBytes(mem.Shared))
		}
		if mem.Buffered > 0 {
			row(Msg("InfoPanel.MemBuffered"), formatBytes(mem.Buffered))
		}
		if mem.SwapTotal > 0 {
			row(Msg("InfoPanel.SwapTotal"), formatBytes(mem.SwapTotal))
			row(Msg("InfoPanel.SwapFree"), formatBytes(mem.SwapFree))
		}
	}
}

// writeLine pads text with trailing spaces to width so the whole row
// uses one attribute — no visible edges of the string on differently
// coloured background.
func (ip *InfoPanel) writeLine(scr *vtui.ScreenBuf, text string, attr uint64, width, y int) {
	pad := width - runewidth.StringWidth(text)
	if pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	scr.Write(ip.X1+1, y, vtui.StringToCharInfo(text, attr))
}

// formatBytesCommas renders a byte count with thousand separators
// (thin non-breaking space, matching far2l's InfoList presentation).
// Kept public inside main so tests can pin the exact string form.
func formatBytesCommas(b uint64) string {
	s := fmt.Sprintf("%d", b)
	if len(s) <= 3 {
		return s
	}
	// Insert a separator every three digits from the right.
	sep := " " // non-breaking space
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, sep...)
		}
		out = append(out, c)
	}
	return string(out)
}

// shortUsername strips the machine/domain prefix from Windows-style
// user names (`INBOOK\sogonov` → `sogonov`), matching how the
// original Far2 InfoList renders it. On Unix `user.Current().Username`
// is already the bare login, so this is a no-op there.
func shortUsername(u string) string {
	if i := strings.LastIndexAny(u, `\/`); i >= 0 {
		return u[i+1:]
	}
	return u
}

// formatBytesHuman renders a byte count in binary units (KiB/MiB/…).
func formatBytesHuman(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatBytes picks between the raw far2l-style form and the human
// form based on AppConfig. Toggled at runtime by pressing `B` while
// the info panel is visible.
func formatBytes(b uint64) string {
	if AppConfig.InfoPanelBytes {
		return formatBytesCommas(b)
	}
	return formatBytesHuman(b)
}

// Compile-time interface check.
var _ AltPanel = (*InfoPanel)(nil)
