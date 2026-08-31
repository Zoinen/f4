package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/textlayout"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

func seededTerminalSemanticHistory(t testing.TB, width, height, rows int) *TerminalView {
	t.Helper()
	var content strings.Builder
	content.Grow(rows * 32)
	for row := 0; row < rows; row++ {
		fmt.Fprintf(&content, "row-%06d payload\n", row)
	}

	tv := NewTerminalView(width, height)
	data := []byte(content.String())
	tv.mu.Lock()
	tv.pt = piecetable.New(data)
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(width)
	tv.styles = []StyleChange{{Offset: 0, Attr: DefaultTermAttr}}
	tv.lastAttr = DefaultTermAttr
	tv.GridHistory = nil
	tv.GridHistoryWrap = nil
	tv.semanticFollowTail = true
	tv.mu.Unlock()
	return tv
}

func terminalModelText(rows []map[string]any) string {
	var out strings.Builder
	for _, row := range rows {
		for _, run := range appMapSlice(row["runs"]) {
			out.WriteString(semanticString(run["text"]))
		}
		out.WriteByte('\n')
	}
	return out.String()
}

func TestTerminalSemanticWindowBoundsLargeHistoryAndDoesNotDuplicateRows(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 100, 30, 100_000)
	defer tv.Close()
	target := vtui.SemanticID(tv)

	if !tv.handleSemanticAction(map[string]any{
		"target": target, "action": "terminal.viewport", "rows": 24,
	}) {
		t.Fatal("terminal viewport action was not handled")
	}
	model := tv.semanticModel(nil)
	if model.ContentExtent != 100_030 {
		t.Fatalf("content extent=%d, want 100000 history rows + 30-row active grid",
			model.ContentExtent)
	}
	if got, limit := len(model.WindowRows), 3*model.ViewportRows; got > limit {
		t.Fatalf("bounded tail window rows=%d, limit %d", got, limit)
	} else if got < model.ViewportRows {
		t.Fatalf("bounded tail window rows=%d, below viewport %d",
			got, model.ViewportRows)
	}
	if model.ViewportStart != model.ContentExtent-int64(model.ViewportRows) {
		t.Fatalf("tail viewport start=%d extent=%d rows=%d",
			model.ViewportStart, model.ContentExtent, model.ViewportRows)
	}
	mapped := model.ToMap()
	if _, duplicated := mapped["rows"]; duplicated {
		t.Fatal("bounded terminal serialized a duplicate legacy rows payload")
	}
	if got := len(appMapSlice(mapped["windowRows"])); got != len(model.WindowRows) {
		t.Fatalf("serialized window rows=%d, want %d", got, len(model.WindowRows))
	}
	if !strings.Contains(terminalModelText(appMapSlice(mapped["windowRows"])),
		"row-099999 payload") {
		t.Fatal("last history row is absent from the initial tail window")
	}
}

func TestTerminalSemanticWindowScrollPinsWhileLiveOutputContinues(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 80, 20, 10_000)
	defer tv.Close()
	target := vtui.SemanticID(tv)
	tv.handleSemanticAction(map[string]any{
		"target": target, "action": "terminal.viewport", "rows": 16,
	})
	if !tv.handleSemanticAction(map[string]any{
		"target": target, "action": "terminal.scroll",
		"visualRow": 4321, "followTail": false,
		"generation": uint64(1),
	}) {
		t.Fatal("terminal scroll action was not handled")
	}
	first := tv.semanticModel(nil).ToMap()
	if got := appInt64(first["viewportStart"]); got != 4321 {
		t.Fatalf("pinned viewport=%d, want 4321", got)
	}
	if appBool(first["followTail"]) {
		t.Fatal("manually scrolled terminal still reports follow-tail")
	}

	parser := NewAnsiParser(tv, nil)
	parser.Process([]byte("live-a\r\nlive-b\r\nlive-c\r\n"))
	second := tv.semanticModel(nil).ToMap()
	if got := appInt64(second["viewportStart"]); got != 4321 {
		t.Fatalf("live output moved pinned viewport to %d", got)
	}
	if overlap := assertSemanticOverlapStable(t, first, second, "rows"); overlap != len(appMapSlice(first["windowRows"])) {
		t.Fatalf("pinned live output rewrote %d of %d stable rows",
			len(appMapSlice(first["windowRows"]))-overlap,
			len(appMapSlice(first["windowRows"])))
	}

	// The UI can see the old final row with a taller or fractional native
	// viewport, while the core's integer top is still before maxTop. Output may
	// also arrive between that observation and this request. The explicit
	// intent must win over both stale coordinates and viewport rounding.
	oldVisibleTop := semanticInt(second["contentExtent"]) -
		semanticInt(second["viewportRows"]) - 1
	parser.Process([]byte("raced-before-tail-request\r\n"))
	if !tv.handleSemanticAction(map[string]any{
		"target": target, "action": "terminal.scroll",
		"visualRow": oldVisibleTop, "followTail": true,
		"generation": uint64(2),
	}) {
		t.Fatal("terminal tail action was not handled")
	}
	beforeTail := tv.semanticModel(nil)
	if !beforeTail.FollowTail {
		t.Fatal("explicit tail request did not restore follow-tail")
	}
	if beforeTail.ViewportStart != beforeTail.ContentExtent-int64(beforeTail.ViewportRows) {
		t.Fatalf("stale tail request stopped at %d, want current max %d",
			beforeTail.ViewportStart,
			beforeTail.ContentExtent-int64(beforeTail.ViewportRows))
	}
	parser.Process([]byte("live-tail\r\n"))
	afterTail := tv.semanticModel(nil)
	if afterTail.ViewportStart <= beforeTail.ViewportStart {
		t.Fatalf("follow-tail viewport did not advance: %d -> %d",
			beforeTail.ViewportStart, afterTail.ViewportStart)
	}

	if !tv.handleSemanticAction(map[string]any{
		"target": target, "action": "terminal.followTail",
		"followTail": false,
	}) {
		t.Fatal("immediate follow-tail suspension was not handled")
	}
	pinned := tv.semanticModel(nil)
	parser.Process([]byte("must-not-pull-user-to-tail\r\n"))
	if got := tv.semanticModel(nil).ViewportStart; got != pinned.ViewportStart {
		t.Fatalf("output moved immediately suspended viewport: %d -> %d",
			pinned.ViewportStart, got)
	}
}

func TestTerminalSemanticSelectionReadsHistoryWithoutMaterializingIt(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 5, 3, 0)
	defer tv.Close()

	tv.mu.Lock()
	tv.pt = piecetable.New([]byte("alpha\nbeta\ngamma\n"))
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(5)
	tv.mu.Unlock()

	if got, want := tv.semanticSelectionText(0, 1, 1, 2), "lpha\nbet"; got != want {
		t.Fatalf("multi-row selection=%q, want %q", got, want)
	}

	tv.mu.Lock()
	tv.pt = piecetable.New([]byte("abcdefghij"))
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(5)
	tv.mu.Unlock()
	if got, want := tv.semanticSelectionText(0, 2, 1, 2), "cdefgh"; got != want {
		t.Fatalf("soft-wrapped selection=%q, want %q", got, want)
	}
}

func TestTerminalSemanticBoundarySelectionUsesInsertionEndpoints(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 20, 3, 0)
	defer tv.Close()
	tv.mu.Lock()
	tv.pt = piecetable.New([]byte("abcdef\nghij\n"))
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(tv.Width)
	tv.mu.Unlock()

	if got, want := tv.semanticBoundarySelectionText(0, 2, 0, 5), "cde"; got != want {
		t.Fatalf("same-row boundary selection=%q, want %q", got, want)
	}
	if got, want := tv.semanticBoundarySelectionText(0, 5, 0, 2), "cde"; got != want {
		t.Fatalf("reversed boundary selection=%q, want %q", got, want)
	}
	if got, want := tv.semanticBoundarySelectionText(0, 2, 1, 2), "cdef\ngh"; got != want {
		t.Fatalf("hard-wrapped boundary selection=%q, want %q", got, want)
	}

	tv.mu.Lock()
	tv.pt = piecetable.New([]byte("abcdefghij"))
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(5)
	tv.Width = 5
	tv.mu.Unlock()
	if got, want := tv.semanticBoundarySelectionText(0, 2, 1, 3), "cdefgh"; got != want {
		t.Fatalf("soft-wrapped boundary selection=%q, want %q", got, want)
	}
}

func TestTerminalSemanticRowsPublishCompleteParagraphSpan(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 5, 3, 0)
	defer tv.Close()
	tv.mu.Lock()
	tv.pt = piecetable.New([]byte("abcdefghij\n"))
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(tv.Width)
	tv.semanticViewportRows = 8
	tv.semanticFollowTail = false
	tv.semanticScrollTop = 0
	tv.mu.Unlock()

	mapped := tv.semanticModel(nil).ToMap()
	if got := semanticInt(mapped["columns"]); got != 5 {
		t.Fatalf("terminal columns=%d, want 5", got)
	}
	rows := appMapSlice(mapped["windowRows"])
	for visualRow := 0; visualRow < 2; visualRow++ {
		var found map[string]any
		for _, row := range rows {
			if semanticInt(row["visualRow"]) == visualRow {
				found = row
				break
			}
		}
		if found == nil {
			t.Fatalf("visual row %d was not published", visualRow)
		}
		if start, end := semanticInt(found["logicalRowStart"]),
			semanticInt(found["logicalRowEnd"]); start != 0 || end != 2 {
			t.Fatalf("visual row %d paragraph span=[%d,%d), want [0,2)",
				visualRow, start, end)
		}
	}
}

func TestTerminalSemanticCopySelectionWritesClipboard(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 20, 3, 0)
	defer tv.Close()
	tv.mu.Lock()
	tv.pt = piecetable.New([]byte("copy this line\n"))
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(tv.Width)
	tv.mu.Unlock()

	copied := make(chan string, 1)
	tv.clipboardWriter = func(text string) { copied <- text }
	if !tv.handleSemanticAction(map[string]any{
		"target": vtui.SemanticID(tv), "action": "terminal.copySelection",
		"startRow": 0, "startColumn": 5, "endRow": 0, "endColumn": 8,
	}) {
		t.Fatal("terminal clipboard action was not handled")
	}
	select {
	case got := <-copied:
		if got != "this" {
			t.Fatalf("clipboard text=%q, want %q", got, "this")
		}
	case <-time.After(time.Second):
		t.Fatal("terminal clipboard write did not complete")
	}
}

func TestTerminalSemanticCopySelectionAcceptsExclusiveBoundaryProtocol(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(t, 20, 3, 0)
	defer tv.Close()
	tv.mu.Lock()
	tv.pt = piecetable.New([]byte("copy this line\n"))
	tv.li = piecetable.NewLineIndex()
	tv.li.Rebuild(tv.pt)
	tv.engine = textlayout.NewWrapEngine(tv.pt, tv.li)
	tv.engine.SetWidth(tv.Width)
	tv.mu.Unlock()

	copied := make(chan string, 1)
	tv.clipboardWriter = func(text string) { copied <- text }
	if !tv.handleSemanticAction(map[string]any{
		"target": vtui.SemanticID(tv), "action": "terminal.copySelection",
		"startRow": 0, "startColumn": 5, "endRow": 0, "endColumn": 9,
		"endExclusive": true,
	}) {
		t.Fatal("exclusive terminal clipboard action was not handled")
	}
	select {
	case got := <-copied:
		if got != "this" {
			t.Fatalf("exclusive clipboard text=%q, want %q", got, "this")
		}
	case <-time.After(time.Second):
		t.Fatal("exclusive terminal clipboard write did not complete")
	}
}

func TestTerminalSemanticCurrentScreenKeepsConsoleBottomGravity(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := NewTerminalView(12, 5)
	defer tv.Close()
	tv.mu.Lock()
	tv.CursorX = 4
	tv.CursorY = 0
	for column, char := range "line" {
		tv.Lines[0][column] = vtui.CharInfo{
			Char: uint64(char), Attributes: DefaultTermAttr,
		}
	}
	tv.semanticViewportRows = 5
	tv.mu.Unlock()

	model := tv.semanticModel(nil)
	if got := model.ContentExtent; got != 5 {
		t.Fatalf("active terminal extent=%d, want one five-row screen", got)
	}
	if got := model.CursorAbsoluteRow; got != 4 {
		t.Fatalf("bottom-gravity cursor row=%d, want 4", got)
	}
	if got := terminalModelText(appMapSlice(model.ToMap()["windowRows"])); !strings.HasSuffix(got, "line\n") {
		t.Fatalf("bottom-gravity rows do not end in current output: %q", got)
	}
}

func TestTerminalSemanticCommandLineOverlayOmitsIdlePromptAndCursor(t *testing.T) {
	vtui.SetDefaultPalette()
	tv := NewTerminalView(24, 5)
	defer tv.Close()
	tv.SetVisible(true)
	tv.SetFocus(true)
	tv.mu.Lock()
	tv.CursorX = len("shell prompt % ")
	tv.CursorY = 1
	for column, char := range "command output" {
		tv.Lines[0][column] = vtui.CharInfo{
			Char: uint64(char), Attributes: DefaultTermAttr,
		}
	}
	for column, char := range "shell prompt % " {
		tv.Lines[1][column] = vtui.CharInfo{
			Char: uint64(char), Attributes: DefaultTermAttr,
		}
	}
	tv.semanticViewportRows = 5
	tv.mu.Unlock()

	withCommandLine := tv.semanticModelWithBottomOverlay(nil, 1)
	if got := withCommandLine.ContentExtent; got != 4 {
		t.Fatalf("content extent with command-line overlay=%d, want 4", got)
	}
	text := terminalModelText(appMapSlice(
		withCommandLine.ToMap()["windowRows"]))
	if !strings.Contains(text, "command output") {
		t.Fatalf("command output disappeared with bottom overlay: %q", text)
	}
	if strings.Contains(text, "shell prompt") {
		t.Fatalf("covered idle shell prompt leaked into terminal history: %q", text)
	}
	if withCommandLine.CursorVisible {
		t.Fatal("cursor on the covered prompt row remained visible")
	}

	withoutCommandLine := tv.semanticModelWithBottomOverlay(nil, 0)
	if got := withoutCommandLine.ContentExtent; got != 5 {
		t.Fatalf("content extent without command-line overlay=%d, want 5", got)
	}
	text = terminalModelText(appMapSlice(
		withoutCommandLine.ToMap()["windowRows"]))
	if !strings.Contains(text, "shell prompt") {
		t.Fatalf("idle shell prompt was not restored without overlay: %q", text)
	}
}

func TestPanelsSemanticProjectionAppliesVisibleCommandLineOverlay(t *testing.T) {
	vtui.SetDefaultPalette()
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(40, 8)

	pf.termView.mu.Lock()
	for row := range pf.termView.Lines {
		for column := range pf.termView.Lines[row] {
			pf.termView.Lines[row][column] = vtui.CharInfo{
				Char: ' ', Attributes: DefaultTermAttr,
			}
		}
	}
	pf.termView.CursorY = 1
	pf.termView.CursorX = len("idle prompt % ")
	for column, char := range "visible output" {
		pf.termView.Lines[0][column] = vtui.CharInfo{
			Char: uint64(char), Attributes: DefaultTermAttr,
		}
	}
	for column, char := range "idle prompt % " {
		pf.termView.Lines[1][column] = vtui.CharInfo{
			Char: uint64(char), Attributes: DefaultTermAttr,
		}
	}
	pf.termView.mu.Unlock()

	pf.cmdLine.SetVisible(true)
	terminal := appMap(pf.SemanticNode(nil)["terminal"])
	text := terminalModelText(appMapSlice(terminal["windowRows"]))
	if strings.Contains(text, "idle prompt") {
		t.Fatalf("visible command line did not cover terminal prompt: %q", text)
	}
	if !strings.Contains(text, "visible output") {
		t.Fatalf("visible output disappeared with command line: %q", text)
	}

	pf.cmdLine.SetVisible(false)
	terminal = appMap(pf.SemanticNode(nil)["terminal"])
	text = terminalModelText(appMapSlice(terminal["windowRows"]))
	if !strings.Contains(text, "idle prompt") {
		t.Fatalf("hidden command line did not restore terminal prompt: %q", text)
	}
}

func BenchmarkTerminalSemanticWindow100K(b *testing.B) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(b, 120, 40, 100_000)
	defer tv.Close()
	tv.handleSemanticAction(map[string]any{
		"target": vtui.SemanticID(tv), "action": "terminal.viewport", "rows": 36,
	})
	// Warm the canonical WrapEngine index. The benchmark measures repeated
	// semantic publication, which must remain O(viewport), not O(history).
	_ = tv.semanticModel(nil)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		model := tv.semanticModel(nil)
		if len(model.WindowRows) == 0 {
			b.Fatal("empty terminal window")
		}
	}
}

func BenchmarkTerminalLiveBurstWith100KScrollback(b *testing.B) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(b, 120, 40, 100_000)
	defer tv.Close()
	tv.handleSemanticAction(map[string]any{
		"target": vtui.SemanticID(tv), "action": "terminal.viewport", "rows": 36,
	})
	parser := NewAnsiParser(tv, nil)
	var burst strings.Builder
	for row := 0; row < 128; row++ {
		fmt.Fprintf(&burst, "live-%04d abcdefghijklmnopqrstuvwxyz\r\n", row)
	}
	data := []byte(burst.String())
	_ = tv.semanticModel(nil)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		parser.Process(data)
		_ = tv.semanticModel(nil)
	}
}

func BenchmarkTerminalLiveWireWith100KScrollback(b *testing.B) {
	vtui.SetDefaultPalette()
	tv := seededTerminalSemanticHistory(b, 120, 40, 100_000)
	defer tv.Close()
	tv.handleSemanticAction(map[string]any{
		"target": vtui.SemanticID(tv), "action": "terminal.viewport", "rows": 36,
	})
	parser := NewAnsiParser(tv, nil)
	var burst strings.Builder
	for row := 0; row < 900; row++ {
		fmt.Fprintf(&burst, "live-%04d abcdefghijklmnopqrstuvwxyz\r\n", row)
	}
	data := []byte(burst.String())
	_ = tv.semanticModel(nil).ToMap()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		parser.Process(data)
		payload, err := msgpack.Marshal(tv.semanticModel(nil).ToMap())
		if err != nil || len(payload) == 0 {
			b.Fatalf("bounded terminal wire publication failed: %v", err)
		}
	}
}
