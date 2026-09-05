package main

import (
	"fmt"
	"os"
	"os/user"
	"reflect"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
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
	// scrollTop is the first logical row rendered inside the frame. Rows are
	// built in full even when the panel is short, so navigation can still reach
	// device details below the current viewport.
	scrollTop int
	// selection persists across rebuilds. Keyed by "section|label"
	// because labels alone collide (CPU and GPU both use i18n key
	// InfoPanel.*Model = "Model"). Values fluctuate (Load %, free
	// bytes) — losing a selection when a value changes is worse
	// than keying by section+label and accepting that the copied
	// value reflects the current sample, which is what the user
	// sees.
	selection map[string]bool

	// Provider information is refreshed outside Show: that method runs on the
	// UI thread and must never wait for a remote filesystem or device. The task
	// pointer and stable provider key form a generation token, so a late Android
	// response cannot repaint an info panel that now mirrors another VFS/device.
	infoTask       *vtui.TaskContext
	infoKey        string
	infoSource     vfs.VFS
	infoProvider   vfs.PanelInfoProvider
	infoSnapshot   vfs.PanelInfoSnapshot
	infoHasRefresh bool
	infoRetryAfter time.Time
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
	// usageBarStart/Width address the bar in screen cells within
	// text. The renderer gives the filled and unfilled portions distinct
	// backgrounds, including percentage characters that cross the boundary.
	usageBarStart  int
	usageBarWidth  int
	usageBarFilled int
	// usageMeterWidth is the complete bracketless bar width. The two-line
	// reusable meter keeps its source values here so a post-layout pass can give every meter in the
	// complete information panel the same minimum width and right edge.
	usageMeterWidth   int
	usageTotal        uint64
	usageAvailable    uint64
	usageContinuation bool
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

const infoPanelRefreshRetryDelay = 5 * time.Second
const infoPanelRefreshDebounce = 75 * time.Millisecond

// sameInfoObject compares the concrete identity behind an interface without
// assuming every third-party VFS/provider has a comparable dynamic type.
// Pointer implementations are the normal case; comparable value providers are
// supported too. An exotic non-comparable value is treated as changed, which
// is conservative: it may restart a refresh but can never accept stale data.
func sameInfoObject(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	at, bt := reflect.TypeOf(a), reflect.TypeOf(b)
	if at != bt {
		return false
	}
	if at.Comparable() {
		return reflect.ValueOf(a).Interface() == reflect.ValueOf(b).Interface()
	}
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	switch av.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return av.Pointer() == bv.Pointer()
	default:
		return false
	}
}

func panelInfoSnapshotPopulated(snapshot vfs.PanelInfoSnapshot) bool {
	return len(snapshot.Sections) != 0 || !snapshot.RefreshedAt.IsZero()
}

func (ip *InfoPanel) cancelInfoRefresh() {
	if task := ip.infoTask; task != nil {
		ip.infoTask = nil
		task.Cancel()
	}
}

// Close is called by PanelsFrame when Ctrl+L hides or replaces the panel.
// Invalidate first so a completion already queued on the UI thread is stale.
func (ip *InfoPanel) Close() {
	ip.cancelInfoRefresh()
	ip.infoKey = ""
	ip.infoSource = nil
	ip.infoProvider = nil
	ip.infoSnapshot = vfs.PanelInfoSnapshot{}
	ip.infoHasRefresh = false
}

// currentProviderInfo returns an immediately renderable cached snapshot and,
// when necessary, starts one cancellable background refresh. Provider cache
// reads are explicitly part of the non-blocking VFS contract; all actual I/O
// stays in RefreshPanelInfo below.
func (ip *InfoPanel) currentProviderInfo() (vfs.PanelInfoSnapshot, bool) {
	if ip.src == nil || ip.src.vfs == nil {
		ip.Close()
		return vfs.PanelInfoSnapshot{}, false
	}
	provider, ok := ip.src.vfs.(vfs.PanelInfoProvider)
	if !ok {
		ip.Close()
		return vfs.PanelInfoSnapshot{}, false
	}

	source := ip.src.vfs
	selected := ip.src.getRawSelectedName()
	req := vfs.PanelInfoRequest{Path: ip.src.vfs.GetPath(), SelectedName: selected}
	key := provider.PanelInfoKey(req)
	if key == "" {
		ip.Close()
		return vfs.PanelInfoSnapshot{}, false
	}

	cached, fresh := provider.CachedPanelInfo(req)
	generationChanged := key != ip.infoKey ||
		!sameInfoObject(ip.infoSource, source) || !sameInfoObject(ip.infoProvider, provider)
	if generationChanged {
		ip.cancelInfoRefresh()
		ip.infoKey = key
		ip.infoSource = source
		ip.infoProvider = provider
		ip.infoSnapshot = cached
		ip.infoHasRefresh = false
		ip.infoRetryAfter = time.Time{}
	} else if fresh {
		// A fresh cache value is authoritative, including an intentionally empty
		// snapshot used to clear rows.
		ip.infoSnapshot = cached
		ip.infoHasRefresh = false
	} else if !ip.infoHasRefresh && panelInfoSnapshotPopulated(cached) {
		// CachedPanelInfo is the provider's authoritative local snapshot. Always
		// take a populated copy until this view has a newer direct refresh result.
		ip.infoSnapshot = cached
	} else if panelInfoSnapshotPopulated(cached) &&
		!cached.RefreshedAt.IsZero() &&
		(ip.infoSnapshot.RefreshedAt.IsZero() || cached.RefreshedAt.After(ip.infoSnapshot.RefreshedAt)) {
		// A shared provider cache refreshed by another view can supersede our
		// result when it carries an explicitly newer timestamp.
		ip.infoSnapshot = cached
	}

	if !fresh && ip.infoTask == nil && !time.Now().Before(ip.infoRetryAfter) {
		refreshKey := key
		refreshSource := source
		refreshProvider := provider
		ip.infoTask = vtui.RunAsync(func(ctx *vtui.TaskContext) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(infoPanelRefreshDebounce):
			}
			snapshot, err := provider.RefreshPanelInfo(ctx.Context, req)
			ctx.RunOnUI(func() {
				if ip.infoTask != ctx || ip.infoKey != refreshKey ||
					!sameInfoObject(ip.infoSource, refreshSource) ||
					!sameInfoObject(ip.infoProvider, refreshProvider) {
					return
				}
				ip.infoTask = nil
				if err != nil {
					ip.infoRetryAfter = time.Now().Add(infoPanelRefreshRetryDelay)
					vtui.DebugLog("INFO: provider refresh %q failed: %v", refreshKey, err)
				} else {
					ip.infoSnapshot = snapshot
					ip.infoHasRefresh = true
					// Providers normally publish the result into CachedPanelInfo. If
					// one only returns it, keep the useful snapshot without starting a
					// new remote request on every redraw.
					ip.infoRetryAfter = time.Now().Add(infoPanelRefreshRetryDelay)
				}
				vtui.FrameManager.Redraw()
			})
		})
	}

	return ip.infoSnapshot, true
}

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
	case vtinput.VK_PRIOR:
		if shift {
			ip.toggleSelectionAtCursor()
		}
		ip.moveCursorPage(-1)
	case vtinput.VK_NEXT:
		if shift {
			ip.toggleSelectionAtCursor()
		}
		ip.moveCursorPage(+1)
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

// moveCursorPage moves by roughly one visible page of physical rows while
// still landing only on copyable rows. Show adjusts scrollTop around the new
// logical cursor on the following redraw.
func (ip *InfoPanel) moveCursorPage(direction int) {
	if len(ip.rows) == 0 || direction == 0 {
		return
	}
	if ip.cursor < 0 {
		ip.setCursorToFirstCopyable()
		return
	}
	page := ip.Y2 - ip.Y1 - 2
	if page < 1 {
		page = 1
	}
	target := ip.cursor + direction*page
	if target < 0 {
		target = 0
	}
	if target >= len(ip.rows) {
		target = len(ip.rows) - 1
	}
	for i := target; i >= 0 && i < len(ip.rows); i += direction {
		if ip.rows[i].copyable {
			ip.cursor = i
			return
		}
	}
	if direction < 0 {
		ip.setCursorToFirstCopyable()
	} else {
		ip.setCursorToLastCopyable()
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

// setCursorToNearestCopyable keeps the cursor near its previous logical
// position when a cached provider baseline and its complete snapshot do not
// contain exactly the same fields. Prefer the row at/after the old position,
// then the equally distant row before it.
func (ip *InfoPanel) setCursorToNearestCopyable(index int) bool {
	if len(ip.rows) == 0 {
		ip.cursor = -1
		return false
	}
	if index < 0 {
		index = 0
	}
	if index >= len(ip.rows) {
		index = len(ip.rows) - 1
	}
	for distance := 0; distance < len(ip.rows); distance++ {
		forward := index + distance
		if forward < len(ip.rows) && ip.rows[forward].copyable {
			ip.cursor = forward
			return true
		}
		backward := index - distance
		if distance != 0 && backward >= 0 && ip.rows[backward].copyable {
			ip.cursor = backward
			return true
		}
	}
	ip.cursor = -1
	return false
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
		showToast(fmt.Sprintf("%s: %s", Msg("InfoPanel.Copied"), r.value), 2*time.Second)
		return
	}
	var lines []string
	for _, r := range selRows {
		lines = append(lines, r.label+": "+r.value)
	}
	joined := strings.Join(lines, "\n")
	vtui.SetClipboard(joined)
	showToast(fmt.Sprintf("%s: %d", Msg("InfoPanel.CopiedRows"), len(selRows)), 2*time.Second)
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
	previousCursorKey := ""
	previousCursorIndex := -1
	previousCursorOffset := -1
	if ip.cursor >= 0 && ip.cursor < len(ip.rows) && ip.rows[ip.cursor].copyable {
		previousCursorKey = rowKey(ip.rows[ip.cursor].section, ip.rows[ip.cursor].label)
		previousCursorIndex = ip.cursor
		previousCursorOffset = ip.cursor - ip.scrollTop
	}
	ip.rows = ip.rows[:0]

	y := ip.Y1 + 1

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
		fullValue := value
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
		text = runewidth.Truncate(text, innerW, "…")
		ip.rows = append(ip.rows, infoRow{
			section: currentSection,
			label:   label, value: fullValue, copyable: copyable && fullValue != "",
			text: text, y: y,
		})
		y++
	}
	blank := func() {
		ip.rows = append(ip.rows, infoRow{y: y})
		y++
	}
	usageRow := func(label string, total, available uint64) {
		rows := (infoUsageMeter{Label: label, Total: total, Available: available}).rows(
			currentSection, innerW, y)
		ip.rows = append(ip.rows, rows...)
		y += len(rows)
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
				label:   label, value: value, copyable: true,
				text: runewidth.Truncate(labelPad, innerW, "…"), y: y,
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
			if cur == "" {
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
				text:    runewidth.Truncate(hangIndent+cur, innerW, "…"),
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
		currentSection = title
		text := " " + title + " "
		w := runewidth.StringWidth(text)
		line := ""
		if w > innerW {
			line = runewidth.Truncate(text, innerW, "…")
		} else {
			pad := (innerW - w) / 2
			line = strings.Repeat("─", pad) + text + strings.Repeat("─", innerW-pad-w)
		}
		ip.rows = append(ip.rows, infoRow{text: line, y: y})
		y++
	}

	// Header — computer / user.
	providerSnapshot, hasProviderInfo := ip.currentProviderInfo()
	providerText := func(key, fallback, id string) string {
		if key != "" {
			if translated := Msg(key); translated != "" && translated != key && translated != "{"+key+"}" {
				return translated
			}
		}
		if fallback != "" {
			return fallback
		}
		return id
	}

	// An authoritative provider describes the computer behind the current VFS
	// (for example an Android device). Do not mix the local host into that view.
	if !providerSnapshot.Authoritative {
		hostname, _ := os.Hostname()
		username := ""
		if u, err := user.Current(); err == nil {
			username = shortUsername(u.Username)
		}
		row(Msg("InfoPanel.Computer"), hostname, true)
		row(Msg("InfoPanel.User"), username, true)
		blank()
	}

	if hasProviderInfo {
		renderedSection := false
		for _, section := range providerSnapshot.Sections {
			if len(section.Fields) == 0 {
				continue
			}
			if renderedSection {
				blank()
			}
			sectionHeader(providerText(section.TitleKey, section.Title, section.ID))
			for _, field := range section.Fields {
				label := providerText(field.LabelKey, field.Label, field.ID)
				value := field.Value
				switch field.Kind {
				case vfs.PanelInfoBytes:
					value = formatBytes(field.Bytes)
					row(label, value, true)
				case vfs.PanelInfoUsage:
					usageRow(label, field.TotalBytes, field.AvailableBytes)
				default:
					row(label, value, true)
				}
			}
			renderedSection = true
		}
		if renderedSection {
			blank()
		}
	}

	// Filesystem.
	path := ""
	if ip.src != nil && ip.src.vfs != nil {
		path = ip.src.vfs.GetPath()
	}
	fsTitle := Msg("InfoPanel.FilesystemTitle")
	if providerSnapshot.Authoritative {
		sectionHeader(fsTitle)
		row(Msg("InfoPanel.CurrentDir"), path, true)
	} else if fs, ok := fsInfo(path); ok {
		if fs.Type != "" {
			fsTitle = fmt.Sprintf("%s (%s)", fsTitle, fs.Type)
		}
		sectionHeader(fsTitle)
		usageRow(Msg("InfoPanel.Space"), fs.Total, fs.Free)
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
	if !providerSnapshot.Authoritative {
		if mem, ok := memInfo(); ok {
			blank()
			sectionHeader(Msg("InfoPanel.MemoryTitle"))
			usageRow(Msg("InfoPanel.Memory"), mem.Total, mem.Free)
			if mem.Shared > 0 {
				row(Msg("InfoPanel.MemShared"), formatBytes(mem.Shared), true)
			}
			if mem.Buffered > 0 {
				row(Msg("InfoPanel.MemBuffered"), formatBytes(mem.Buffered), true)
			}
			if mem.SwapTotal > 0 {
				usageRow(Msg("InfoPanel.PagingFile"), mem.SwapTotal, mem.SwapFree)
			}
		}
	}

	// CPU + GPU — opt-in, off by default (maintainer's ask). Kept
	// after Memory so a user who enables the section doesn't have
	// what they see above shifted downward.
	if !providerSnapshot.Authoritative && AppConfig.InfoPanelCPUGPU {
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

	// Labels vary between sections and languages. Each usage element first
	// computes the widest meter it can naturally accommodate; normalize all of
	// them only after the complete logical row list is known, including rows
	// below the current scroll viewport.
	alignInfoUsageMeters(ip.rows, innerW)

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

	// A cached Android baseline grows several rows when the background probe
	// completes. Preserve the cursor by semantic row identity, not by its old
	// numeric index, so it does not jump to a newly inserted field.
	if previousCursorKey != "" {
		restored := false
		for i := range ip.rows {
			if ip.rows[i].copyable && rowKey(ip.rows[i].section, ip.rows[i].label) == previousCursorKey {
				ip.cursor = i
				if previousCursorOffset >= 0 {
					ip.scrollTop = ip.cursor - previousCursorOffset
				}
				restored = true
				break
			}
		}
		if !restored {
			ip.setCursorToNearestCopyable(previousCursorIndex)
			if ip.cursor >= 0 && previousCursorOffset >= 0 {
				ip.scrollTop = ip.cursor - previousCursorOffset
			}
		}
	}

	// If the cursor is stale (out of range after a resize) or was
	// never placed, seed it on the first copyable row.
	if ip.cursor < 0 || ip.cursor >= len(ip.rows) || !ip.rows[ip.cursor].copyable {
		ip.setCursorToFirstCopyable()
	}

	// Keep the logical cursor visible and clamp a viewport left over from a
	// taller/longer previous snapshot. The row list itself is never clipped:
	// otherwise a short terminal would make the lower device fields impossible
	// to reach after focusing the Info panel with Tab.
	visibleRows := ip.Y2 - ip.Y1 - 1
	if visibleRows < 0 {
		visibleRows = 0
	}
	maxScroll := len(ip.rows) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ip.scrollTop > maxScroll {
		ip.scrollTop = maxScroll
	}
	if ip.scrollTop < 0 {
		ip.scrollTop = 0
	}
	if visibleRows > 0 && ip.cursor >= 0 {
		if ip.cursor < ip.scrollTop {
			ip.scrollTop = ip.cursor
		} else if ip.cursor >= ip.scrollTop+visibleRows {
			ip.scrollTop = ip.cursor - visibleRows + 1
		}
	}

	// Render pass — draws only the current viewport while retaining every row
	// for navigation, selection and copying.
	// Colour picks in order of increasing "attention":
	//   plain → selected → cursor → cursor-on-selected
	// so a selected row you're standing on gets the highest-contrast
	// treatment (matches ColPanelSelectedCursor in the file panel).
	for i := range ip.rows {
		ip.rows[i].y = -1
	}
	end := ip.scrollTop + visibleRows
	if end > len(ip.rows) {
		end = len(ip.rows)
	}
	for i := ip.scrollTop; i < end; i++ {
		r := ip.rows[i]
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
		screenY := ip.Y1 + 1 + i - ip.scrollTop
		ip.rows[i].y = screenY
		ip.writeInfoRow(scr, r, lineAttr, innerW, screenY)
	}
}

func (ip *InfoPanel) writeInfoRow(scr *vtui.ScreenBuf, row infoRow, attr uint64, width, y int) {
	text := runewidth.Truncate(row.text, width, "…")
	pad := width - runewidth.StringWidth(text)
	if pad > 0 {
		text += strings.Repeat(" ", pad)
	}
	cells := vtui.StringToCharInfo(text, attr)
	filledAttr, unfilledAttr := panelInfoUsageAttrs(attr)
	for offset := 0; offset < row.usageBarWidth; offset++ {
		cell := row.usageBarStart + offset
		if cell < 0 || cell >= len(cells) {
			continue
		}
		if offset < row.usageBarFilled {
			cells[cell].Attributes = filledAttr
		} else {
			cells[cell].Attributes = unfilledAttr
		}
	}
	scr.Write(ip.X1+1, y, cells)
}

func panelInfoAttrColors(attr uint64) (foreground, background uint32) {
	if attr&vtui.IsFgRGB != 0 {
		foreground = vtui.GetRGBFore(attr)
	} else {
		foreground = vtui.ThemePalette[vtui.GetIndexFore(attr)]
	}
	if attr&vtui.IsBgRGB != 0 {
		background = vtui.GetRGBBack(attr)
	} else {
		background = vtui.ThemePalette[vtui.GetIndexBack(attr)]
	}
	return foreground, background
}

func panelInfoBlendRGB(base, tint uint32, tintPercent uint32) uint32 {
	if tintPercent > 100 {
		tintPercent = 100
	}
	basePercent := uint32(100) - tintPercent
	blend := func(shift uint32) uint32 {
		baseComponent := (base >> shift) & 0xff
		tintComponent := (tint >> shift) & 0xff
		return (baseComponent*basePercent + tintComponent*tintPercent + 50) / 100
	}
	return blend(16)<<16 | blend(8)<<8 | blend(0)
}

func panelInfoUsageAttrs(attr uint64) (filled, unfilled uint64) {
	foreground, background := panelInfoAttrColors(attr)
	base := attr &^ vtui.CommonLvbReverse
	filled = vtui.SetRGBBoth(base, background, foreground)
	// A subtle tint makes the remaining capacity readable without competing
	// with the bright filled portion of the meter.
	unfilledBackground := panelInfoBlendRGB(background, foreground, 22)
	unfilled = vtui.SetRGBBoth(base, foreground, unfilledBackground)
	return filled, unfilled
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
