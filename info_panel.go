package main

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

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
// current directory, disk space, memory and (when enabled) CPU/GPU.
// Per-file details ("Quick view") belong to Ctrl+Q; git status and
// description-file (README/Descript.ion) rendering are deferred.
type InfoPanel struct {
	vtui.ScreenObject
	src     *FileSystemPanel
	frame   *vtui.BorderedFrame
	focused bool

	// rows is rebuilt on every Show and remembered for hit-testing by
	// ProcessKey (Up/Down/C need to know what's on screen).
	rows []infoRow
	// cursor indexes into rows; only rows with copyable == true are
	// stopping points. Clamped by moveCursor on each navigation call.
	cursor int
	// selection persists across rebuilds. Keyed by "section|label"
	// because labels alone collide (CPU and GPU both use i18n key
	// InfoPanel.*Model = "Model"). Values fluctuate (Load %, free
	// bytes) — losing a selection when a value changes is worse
	// than keying by section+label and accepting that the copied
	// value reflects the current sample, which is what the user
	// sees.
	selection map[string]bool
}

// infoRow captures one rendered line so the highlight (when focused)
// and the C-copies-value command can share what the Show pass built.
// Section headers and blank spacers set copyable=false; navigation
// skips them.
type infoRow struct {
	section  string
	label    string
	value    string
	copyable bool
	// text is the pre-composed label + gap + value line (or the
	// wrapped label-only line for wrapRow's first segment). It's
	// stashed so a redraw of the cursor row doesn't have to
	// recompute alignment.
	text string
	y    int
	// selected is filled from ip.selection on each Show — it's not
	// authoritative, only a rendering hint.
	selected bool
}

// rowKey composes the selection-map key for a row. Empty section
// (Computer/User header before any sectionHeader() call) keeps the
// key well-formed and still unique across sections.
func rowKey(section, label string) string {
	return section + "|" + label
}

// NewInfoPanel creates an info panel positioned over src's slot.
// The caller is expected to reposition it via SetPosition to fit the
// current layout (PanelsFrame.ResizeConsole does this).
func NewInfoPanel(src *FileSystemPanel) *InfoPanel {
	x1, y1, x2, y2 := src.GetPosition()
	ip := &InfoPanel{src: src, cursor: -1, selection: map[string]bool{}}
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

// SetFocus flips the visible focus marker (title recolours). When the
// panel is focused it also starts consuming Up / Down / C keys — see
// ProcessKey.
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

// ProcessKey consumes navigation, selection and C while focused. All
// other keys (B and Ctrl+L in particular) fall through to the global
// handler chain so the units toggle and close behaviour still work.
// Shift+Up/Down mirror the file panel's convention: toggle the
// current row's selection, then move. Ins toggles without moving.
func (ip *InfoPanel) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || !ip.focused {
		return false
	}
	if e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
		return false
	}
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0

	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		if shift {
			ip.toggleSelectionAtCursor()
		}
		ip.moveCursor(-1)
	case vtinput.VK_DOWN:
		if shift {
			ip.toggleSelectionAtCursor()
		}
		ip.moveCursor(+1)
	case vtinput.VK_HOME:
		if shift {
			ip.toggleSelectionAtCursor()
		}
		ip.setCursorToFirstCopyable()
	case vtinput.VK_END:
		if shift {
			ip.toggleSelectionAtCursor()
		}
		ip.setCursorToLastCopyable()
	case vtinput.VK_INSERT:
		if shift {
			return false
		}
		ip.toggleSelectionAtCursor()
		ip.moveCursor(+1)
	case vtinput.VK_C:
		if shift {
			return false
		}
		ip.copyCurrent()
	default:
		return false
	}
	vtui.FrameManager.HardRefresh()
	return true
}

// toggleSelectionAtCursor flips the persistent selection bit for the
// row under the cursor. No-op on non-copyable rows (headers, blanks)
// so the user doesn't have to worry about invisible state.
func (ip *InfoPanel) toggleSelectionAtCursor() {
	if ip.cursor < 0 || ip.cursor >= len(ip.rows) {
		return
	}
	r := &ip.rows[ip.cursor]
	if !r.copyable {
		return
	}
	key := rowKey(r.section, r.label)
	if ip.selection[key] {
		delete(ip.selection, key)
		r.selected = false
	} else {
		ip.selection[key] = true
		r.selected = true
	}
}

// ProcessMouse: alt panels don't handle clicks — fall through.
func (ip *InfoPanel) ProcessMouse(*vtinput.InputEvent) bool { return false }

// GetSelectedName proxies to the source so callers that inspect
// "the passive panel's selection" (drive menu, etc.) keep working.
func (ip *InfoPanel) GetSelectedName() string {
	if ip.src == nil {
		return ""
	}
	return ip.src.GetSelectedName()
}

// moveCursor advances to the next copyable row in the given
// direction (+1 or -1), skipping section headers and blank lines.
// Clamps at the ends — no wrap-around, matches how the file panel
// treats Up/Down at the extremes.
func (ip *InfoPanel) moveCursor(delta int) {
	if len(ip.rows) == 0 {
		return
	}
	if ip.cursor < 0 {
		ip.setCursorToFirstCopyable()
		return
	}
	i := ip.cursor
	for {
		i += delta
		if i < 0 || i >= len(ip.rows) {
			return
		}
		if ip.rows[i].copyable {
			ip.cursor = i
			return
		}
	}
}

func (ip *InfoPanel) setCursorToFirstCopyable() {
	for i, r := range ip.rows {
		if r.copyable {
			ip.cursor = i
			return
		}
	}
}

func (ip *InfoPanel) setCursorToLastCopyable() {
	for i := len(ip.rows) - 1; i >= 0; i-- {
		if ip.rows[i].copyable {
			ip.cursor = i
			return
		}
	}
}

// copyCurrent copies the row(s) under the C hotkey to the clipboard.
//
//   - With at least one row selected via Shift+Up/Down or Ins:
//     joins every selected row as "label: value" per line, in the
//     order they appear on screen. Toast shows "Copied N rows".
//   - Otherwise: copies the current row's raw value (no label),
//     which is what single-row-copy has always done.
//
// vtui.SetClipboard already tries far2l IPC → OS clipboard →
// OSC 52 in order, so a single call covers every terminal case f4
// supports.
func (ip *InfoPanel) copyCurrent() {
	var selRows []infoRow
	for _, r := range ip.rows {
		if r.selected && r.copyable && r.value != "" {
			selRows = append(selRows, r)
		}
	}
	if len(selRows) == 0 {
		if ip.cursor < 0 || ip.cursor >= len(ip.rows) {
			return
		}
		r := ip.rows[ip.cursor]
		if !r.copyable || r.value == "" {
			return
		}
		vtui.SetClipboard(r.value)
		vtui.ShowToast(fmt.Sprintf("%s: %s", Msg("InfoPanel.Copied"), r.value), 2*time.Second)
		return
	}
	var lines []string
	for _, r := range selRows {
		lines = append(lines, r.label+": "+r.value)
	}
	joined := strings.Join(lines, "\n")
	vtui.SetClipboard(joined)
	vtui.ShowToast(fmt.Sprintf("%s: %d", Msg("InfoPanel.CopiedRows"), len(selRows)), 2*time.Second)
}

func (ip *InfoPanel) Show(scr *vtui.ScreenBuf) {
	if ip.frame != nil {
		ip.frame.Show(scr)
	}
	// Bottom-border hint reminding the user of the units toggle and
	// the copy shortcut. Drawn on the ┴ line so the panel is
	// self-documenting without a menu entry.
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
	ip.rows = ip.rows[:0]

	y := ip.Y1 + 1
	maxY := ip.Y2 - 1

	// currentSection is updated by sectionHeader; row/wrapRow tag
	// every infoRow they emit with it so the selection map can key
	// by section+label rather than label alone (avoids CPU Model /
	// GPU Model colliding on the same "Model" i18n string).
	currentSection := ""

	// Two-column row: label on the left, value right-aligned. Both
	// use the same attr as the frame background so the row reads as
	// a single flat block, matching far2l's InfoList layout. Returns
	// the composed text so buildRows can stash it.
	row := func(label, value string, copyable bool) {
		if y > maxY {
			return
		}
		labelPad := " " + label
		text := labelPad
		if value != "" {
			labelW := runewidth.StringWidth(labelPad)
			valueW := runewidth.StringWidth(value)
			space := innerW - labelW - valueW
			if space < 1 {
				roomForValue := innerW - labelW - 1
				if roomForValue < 1 {
					text = labelPad
				} else {
					value = runewidth.Truncate(value, roomForValue, "…")
					text = labelPad + " " + value
				}
			} else {
				text = labelPad + strings.Repeat(" ", space) + value
			}
		}
		ip.rows = append(ip.rows, infoRow{
			section: currentSection,
			label:   label, value: value, copyable: copyable && value != "",
			text: text, y: y,
		})
		y++
	}
	blank := func() {
		if y > maxY {
			return
		}
		ip.rows = append(ip.rows, infoRow{y: y})
		y++
	}

	// wrapRow is row for a value too long to share a line with its
	// label — breaks on `sep`, continues on hanging lines. Used for
	// Flags (Windows NTFS attributes exceed 60 cols).
	wrapRow := func(label, value, sep string) {
		labelPad := " " + label
		labelW := runewidth.StringWidth(labelPad)
		fitsInline := labelW+1+runewidth.StringWidth(value) <= innerW
		if fitsInline || value == "" {
			row(label, value, true)
			return
		}
		hangStart := 3
		if hangStart >= innerW {
			hangStart = 1
		}
		hangIndent := strings.Repeat(" ", hangStart)
		hangRoom := innerW - hangStart
		if hangRoom < 1 {
			row(label, value, true)
			return
		}
		parts := strings.Split(value, sep)
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
		// First screen line: label + first-chunk (or the label alone
		// if not even the first token fits). This one carries the
		// copyable flag so the full value can be yanked with C.
		if firstValue == "" {
			ip.rows = append(ip.rows, infoRow{
				section: currentSection,
				label:   label, value: value, copyable: true, text: labelPad, y: y,
			})
			y++
		} else {
			text := labelPad + strings.Repeat(" ", innerW-labelW-runewidth.StringWidth(firstValue)) + firstValue
			ip.rows = append(ip.rows, infoRow{
				section: currentSection,
				label:   label, value: value, copyable: true, text: text, y: y,
			})
			y++
		}
		cur := ""
		flush := func() {
			if cur == "" || y > maxY {
				return
			}
			// Tag continuation lines with the same (section, label)
			// as the parent row so selecting the row highlights the
			// wrap too — the line break is a display artifact, not
			// a semantic boundary. copyable stays false so navigation
			// still skips these and copy doesn't duplicate the value.
			ip.rows = append(ip.rows, infoRow{
				section: currentSection,
				label:   label,
				text:    hangIndent + cur,
				y:       y,
			})
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

	sectionHeader := func(title string) {
		if y > maxY {
			return
		}
		currentSection = title
		text := " " + title + " "
		w := runewidth.StringWidth(text)
		pad := 0
		if w < innerW {
			pad = (innerW - w) / 2
		}
		line := strings.Repeat("─", pad) + text + strings.Repeat("─", innerW-pad-w)
		if w > innerW {
			line = runewidth.Truncate(text, innerW, "…")
		}
		ip.rows = append(ip.rows, infoRow{text: line, y: y})
		y++
	}

	// Header — computer / user.
	hostname, _ := os.Hostname()
	username := ""
	if u, err := user.Current(); err == nil {
		username = shortUsername(u.Username)
	}
	row(Msg("InfoPanel.Computer"), hostname, true)
	row(Msg("InfoPanel.User"), username, true)
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
		row(Msg("InfoPanel.Total"), formatBytes(fs.Total), true)
		row(Msg("InfoPanel.Free"), formatBytes(fs.Free), true)
		if fs.Label != "" {
			row(Msg("InfoPanel.Label"), fs.Label, true)
		}
		if fs.Serial != "" {
			row(Msg("InfoPanel.Serial"), fs.Serial, true)
		}
		row(Msg("InfoPanel.CurrentDir"), path, true)
		if fs.Mount != "" && fs.Mount != path {
			row(Msg("InfoPanel.Mount"), fs.Mount, true)
		}
		if fs.MaxFilename > 0 {
			row(Msg("InfoPanel.MaxFilename"), fmt.Sprintf("%d", fs.MaxFilename), true)
		}
		if fs.Flags != "" {
			wrapRow(Msg("InfoPanel.Flags"), fs.Flags, ",")
		}
	} else {
		sectionHeader(fsTitle)
		row(Msg("InfoPanel.CurrentDir"), path, true)
	}

	// Memory. Same numbers as far2l's InfoList reads via sysinfo(2)
	// on Linux — see mem_info_unix.go for the exact formula.
	if mem, ok := memInfo(); ok {
		blank()
		sectionHeader(Msg("InfoPanel.MemoryTitle"))
		row(Msg("InfoPanel.MemLoad"), fmt.Sprintf("%d%%", mem.LoadPercent), true)
		row(Msg("InfoPanel.MemTotal"), formatBytes(mem.Total), true)
		row(Msg("InfoPanel.MemFree"), formatBytes(mem.Free), true)
		if mem.Shared > 0 {
			row(Msg("InfoPanel.MemShared"), formatBytes(mem.Shared), true)
		}
		if mem.Buffered > 0 {
			row(Msg("InfoPanel.MemBuffered"), formatBytes(mem.Buffered), true)
		}
		if mem.SwapTotal > 0 {
			row(Msg("InfoPanel.SwapTotal"), formatBytes(mem.SwapTotal), true)
			row(Msg("InfoPanel.SwapFree"), formatBytes(mem.SwapFree), true)
		}
	}

	// CPU + GPU — opt-in, off by default (maintainer's ask). Kept
	// after Memory so a user who enables the section doesn't have
	// what they see above shifted downward.
	if AppConfig.InfoPanelCPUGPU {
		if cpu, ok := cpuInfo(); ok {
			blank()
			sectionHeader(Msg("InfoPanel.CPUTitle"))
			if cpu.Model != "" {
				row(Msg("InfoPanel.CPUModel"), cpu.Model, true)
			}
			if cpu.PhysicalCores > 0 && cpu.LogicalCores > 0 && cpu.PhysicalCores != cpu.LogicalCores {
				row(Msg("InfoPanel.CPUCores"),
					fmt.Sprintf("%d / %d", cpu.PhysicalCores, cpu.LogicalCores), true)
			} else if cpu.LogicalCores > 0 {
				row(Msg("InfoPanel.CPUCores"), fmt.Sprintf("%d", cpu.LogicalCores), true)
			}
			if cpu.FreqMHz > 0 {
				row(Msg("InfoPanel.CPUFreq"), formatMHz(cpu.FreqMHz), true)
			}
			for i, sz := range cpu.CacheBytes {
				if sz == 0 {
					continue
				}
				row(fmt.Sprintf("L%d", i+1), formatBytes(sz), true)
			}
			switch {
			case cpu.HasLoadPct:
				row(Msg("InfoPanel.CPULoad"), fmt.Sprintf("%d%%", cpu.Load), true)
			case cpu.HasLoad:
				row(Msg("InfoPanel.CPULoadAvg"),
					fmt.Sprintf("%.2f %.2f %.2f", cpu.LoadAvg[0], cpu.LoadAvg[1], cpu.LoadAvg[2]),
					true)
			}
		}
		if gpus, ok := gpuInfo(); ok {
			blank()
			sectionHeader(Msg("InfoPanel.GPUTitle"))
			for i, g := range gpus {
				label := Msg("InfoPanel.GPUModel")
				if len(gpus) > 1 {
					label = fmt.Sprintf("%s %d", label, i+1)
				}
				row(label, g.Model, true)
				if g.Driver != "" {
					dLabel := Msg("InfoPanel.GPUDriver")
					if len(gpus) > 1 {
						dLabel = fmt.Sprintf("%s %d", dLabel, i+1)
					}
					row(dLabel, g.Driver, true)
				}
			}
		}
	}

	// Restore the persisted selection so the highlight survives a
	// rebuild (which happens on every Show). Keyed by section+label
	// — see InfoPanel.selection docs for the rationale. Applied to
	// every row (not just copyable) so a wrapRow's continuation
	// lines light up alongside their parent when the label is
	// selected.
	for i := range ip.rows {
		if ip.rows[i].label != "" && ip.selection[rowKey(ip.rows[i].section, ip.rows[i].label)] {
			ip.rows[i].selected = true
		}
	}

	// If the cursor is stale (out of range after a resize) or was
	// never placed, seed it on the first copyable row.
	if ip.cursor < 0 || ip.cursor >= len(ip.rows) || !ip.rows[ip.cursor].copyable {
		ip.setCursorToFirstCopyable()
	}

	// Render pass — pushes each row to the screen at its recorded y.
	// Colour picks in order of increasing "attention":
	//   plain → selected → cursor → cursor-on-selected
	// so a selected row you're standing on gets the highest-contrast
	// treatment (matches ColPanelSelectedCursor in the file panel).
	for i, r := range ip.rows {
		lineAttr := attr
		isCursor := ip.focused && i == ip.cursor && r.copyable
		switch {
		case isCursor && r.selected:
			lineAttr = vtui.Palette[ColPanelSelectedCursor]
		case isCursor:
			lineAttr = vtui.Palette[ColPanelCursor]
		case r.selected:
			lineAttr = vtui.Palette[ColPanelSelectedText]
		}
		ip.writeLine(scr, r.text, lineAttr, innerW, r.y)
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
	sep := " " // non-breaking space (thin thousands separator)
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

// formatMHz renders a clock speed as MHz or GHz — GHz for anything
// at or above 1 GHz to match the "3.2 GHz" convention every Task
// Manager / lscpu / About This Mac uses.
func formatMHz(mhz int) string {
	if mhz >= 1000 {
		return fmt.Sprintf("%.2f GHz", float64(mhz)/1000)
	}
	return fmt.Sprintf("%d MHz", mhz)
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
