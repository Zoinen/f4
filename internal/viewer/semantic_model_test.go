package viewer

import (
	context "context"
	fmt "fmt"
	semantic "github.com/unxed/f4/internal/semantic"
	vfs "github.com/unxed/f4/vfs"
	vtui "github.com/unxed/vtui"
	os "os"
	filepath "path/filepath"
	strings "strings"
	testing "testing"
	time "time"
)

func TestSemantic_ViewerViewActions(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "view.txt")
	if err := os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmp)
	viewer, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatalf("failed to create viewer: %v", err)
	}
	defer viewer.Close()

	// Test scroll action
	actionScroll := map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scroll",
		"offset": float64(6), // Starts 'line2'
	}
	if !viewer.HandleSemanticAction(actionScroll) {
		t.Fatal("viewer scroll action was not handled")
	}
	if viewer.TopOffset != 6 {
		t.Errorf("expected TopOffset 6, got %d", viewer.TopOffset)
	}
}

func awaitSemanticViewerWindow(t *testing.T, viewer *ViewerView, minimumRows int) map[string]any {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		node := viewer.SemanticNode(nil)
		if len(semantic.AppMapSlice(node["windowRows"])) >= minimumRows {
			return node
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timed out waiting for semantic viewer window: %#v", node)
		}
	}
}

func TestSemantic_ViewerWindowIsBoundedAndByteAddressed(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "window.txt")
	var content strings.Builder
	var offsets []int64
	for i := 0; i < 80; i++ {
		offsets = append(offsets, int64(content.Len()))
		fmt.Fprintf(&content, "line-%02d %s\n", i, strings.Repeat("x", i%7))
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 39, 8) // Eight document rows below the top bar.
	viewer.TopOffset = offsets[30]
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	node := awaitSemanticViewerWindow(t, viewer, 8)
	windowRows := semantic.AppMapSlice(node["windowRows"])
	visibleRows := semantic.AppMapSlice(node["rows"])
	viewportRows := semantic.Int(node["viewportRows"])
	viewportRow := semantic.Int(node["viewportRow"])
	if semantic.String(node["scrollUnit"]) != "bytes" {
		t.Fatalf("scrollUnit = %q", node["scrollUnit"])
	}
	if viewportRows != 8 || len(visibleRows) != viewportRows {
		t.Fatalf("viewport rows=%d visible=%d", viewportRows, len(visibleRows))
	}
	if len(windowRows) <= viewportRows || len(windowRows) > viewportRows+2*semantic.SemanticWindowBufferRows(viewportRows) {
		t.Fatalf("bounded window rows = %d for viewport %d", len(windowRows), viewportRows)
	}
	if viewportRow < 1 || viewportRow >= len(windowRows) {
		t.Fatalf("viewportRow = %d, window rows = %d", viewportRow, len(windowRows))
	}
	if got := semantic.AppInt64(windowRows[viewportRow]["offset"]); got != viewer.TopOffset {
		t.Fatalf("viewport offset = %d, want %d", got, viewer.TopOffset)
	}
	for i := 0; i+1 < len(windowRows); i++ {
		end := semantic.AppInt64(windowRows[i]["endOffset"])
		next := semantic.AppInt64(windowRows[i+1]["offset"])
		if end != next {
			t.Fatalf("row %d end=%d, next=%d", i, end, next)
		}
	}
	if semantic.AppInt64(node["windowStart"]) > viewer.TopOffset ||
		semantic.AppInt64(node["windowEnd"]) < viewer.TopOffset+semantic.AppInt64(node["viewportSpan"]) ||
		semantic.AppInt64(node["contentExtent"]) != int64(content.Len()) ||
		node["contentExtentKnown"] != true {
		t.Fatalf("invalid viewer window contract: %#v", node)
	}
	if got := semantic.String(node["topBarLeft"]); got != " window.txt" {
		t.Fatalf("viewer top bar left=%q, want %q", got, " window.txt")
	}
	viewerStatus := semantic.String(node["topBarRight"])
	if !strings.Contains(viewerStatus, vfs.DisplayCodepageName(viewer.Codepage)) ||
		!strings.Contains(viewerStatus, "%") {
		t.Fatalf("viewer top bar right=%q does not contain codepage and progress", viewerStatus)
	}

	beforeGeneration := viewer.semanticWindowGeneration
	viewer.TopOffset = 0
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer), "action": "viewer.scroll", "offset": int64(-1),
	}) {
		t.Fatal("clamped viewer scroll was not acknowledged")
	}
	if viewer.TopOffset != 0 || viewer.semanticWindowGeneration != beforeGeneration+1 {
		t.Fatalf("clamped scroll offset=%d generation=%d", viewer.TopOffset,
			viewer.semanticWindowGeneration)
	}
}

func TestSemantic_ViewerTenGiBHexWindowStaysSparseAndInt64Addressed(t *testing.T) {
	vtui.SetDefaultPalette()
	const fileSize int64 = 10 * 1024 * 1024 * 1024
	file := &largeBinaryFile{size: fileSize}
	base := vfs.NewOSVFS(t.TempDir())
	viewer, err := NewViewerView(context.Background(), &singleFileVFS{VFS: base, File: file}, "ten-gib.7z")
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	if !viewer.HexMode {
		t.Fatal("10 GiB binary fixture did not open in hex mode")
	}

	viewer.SetPosition(0, 0, 79, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	const requestedOffset int64 = 7*1024*1024*1024 + 123
	wantTop := requestedOffset &^ int64(0xF)
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": requestedOffset,
	}) {
		t.Fatal("far 64-bit viewer scroll was not handled")
	}
	if viewer.TopOffset != 0 || viewer.semanticWindowGeneration != 0 || !viewer.SemanticPendingScroll {
		t.Fatalf("far request committed before source readiness: top=%d generation=%d pending=%v", viewer.TopOffset, viewer.semanticWindowGeneration, viewer.SemanticPendingScroll)
	}

	// The first snapshot starts one asynchronous cache fill at the far window.
	// Completing that one task must be sufficient; a sequential scan of the
	// preceding seven GiB would either time out or produce additional reads.
	_ = viewer.SemanticNode(nil)
	deadline := time.After(2 * time.Second)
	for {
		viewer.Backend.mu.Lock()
		fetching := viewer.Backend.isFetching
		cacheOff := viewer.Backend.cacheOff
		cacheLen := len(viewer.Backend.cacheData)
		viewer.Backend.mu.Unlock()
		if !fetching && cacheLen > 0 && cacheOff > 6*1024*1024*1024 {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timed out loading sparse far window: off=%d len=%d fetching=%v",
				cacheOff, cacheLen, fetching)
		}
	}

	node := viewer.SemanticNode(nil)
	if viewer.TopOffset != wantTop || viewer.SemanticPendingScroll || viewer.semanticWindowGeneration == 0 {
		t.Fatalf("ready far window was not committed: top=%d generation=%d pending=%v", viewer.TopOffset, viewer.semanticWindowGeneration, viewer.SemanticPendingScroll)
	}
	viewportRows := semantic.Int(node["viewportRows"])
	bufferRows := semantic.SemanticWindowBufferRows(viewportRows)
	windowRows := semantic.AppMapSlice(node["windowRows"])
	if got := semantic.AppInt64(node["contentExtent"]); got != fileSize {
		t.Fatalf("content extent=%d, want exact 10 GiB=%d", got, fileSize)
	}
	if got := semantic.AppInt64(node["size"]); got != fileSize {
		t.Fatalf("surface size=%d, want exact 10 GiB=%d", got, fileSize)
	}
	if node["contentExtentKnown"] != true || semantic.String(node["scrollUnit"]) != "bytes" {
		t.Fatalf("invalid global scrollbar contract: known=%v unit=%q",
			node["contentExtentKnown"], node["scrollUnit"])
	}
	if got := semantic.AppInt64(node["viewportStart"]); got != wantTop {
		t.Fatalf("viewport start=%d, want %d", got, wantTop)
	}
	if viewportRows != 8 {
		t.Fatalf("viewport rows=%d, want 8", viewportRows)
	}
	if len(windowRows) != viewportRows+2*bufferRows {
		t.Fatalf("bounded window rows=%d, want %d (viewport=%d buffer=%d)",
			len(windowRows), viewportRows+2*bufferRows, viewportRows, bufferRows)
	}
	wantWindowStart := wantTop - int64(bufferRows*16)
	wantWindowEnd := wantWindowStart + int64(len(windowRows)*16)
	if got := semantic.AppInt64(node["windowStart"]); got != wantWindowStart {
		t.Fatalf("window start=%d, want %d", got, wantWindowStart)
	}
	if got := semantic.AppInt64(node["windowEnd"]); got != wantWindowEnd {
		t.Fatalf("window end=%d, want %d", got, wantWindowEnd)
	}
	if got := semantic.AppInt64(node["viewportSpan"]); got != int64(viewportRows*16) {
		t.Fatalf("viewport span=%d, want %d", got, viewportRows*16)
	}
	viewportRow := semantic.Int(node["viewportRow"])
	if viewportRow != bufferRows || semantic.AppInt64(windowRows[viewportRow]["offset"]) != wantTop {
		t.Fatalf("viewport row=%d offset=%d, want row=%d offset=%d",
			viewportRow, semantic.AppInt64(windowRows[viewportRow]["offset"]), bufferRows, wantTop)
	}
	for i, row := range windowRows {
		wantOffset := wantWindowStart + int64(i*16)
		if got := semantic.AppInt64(row["offset"]); got != wantOffset {
			t.Fatalf("window row %d offset=%d, want %d", i, got, wantOffset)
		}
		if got := semantic.AppInt64(row["endOffset"]); got != wantOffset+16 {
			t.Fatalf("window row %d end=%d, want %d", i, got, wantOffset+16)
		}
	}

	viewer.Backend.mu.Lock()
	cacheOff := viewer.Backend.cacheOff
	cacheBytes := len(viewer.Backend.cacheData)
	viewer.Backend.mu.Unlock()
	if cacheBytes == 0 || cacheBytes > 256*1024 {
		t.Fatalf("viewer retained %d cache bytes, want 1..256 KiB", cacheBytes)
	}
	if cacheOff > wantWindowStart || cacheOff+int64(cacheBytes) < wantWindowEnd {
		t.Fatalf("cache [%d,%d) does not cover semantic window [%d,%d)",
			cacheOff, cacheOff+int64(cacheBytes), wantWindowStart, wantWindowEnd)
	}

	reads := file.readRanges()
	if len(reads) != 2 {
		t.Fatalf("10 GiB sparse viewer performed %d reads, want header + one far cache fill: %#v",
			len(reads), reads)
	}
	if reads[0].offset != 0 || reads[0].length != 16*1024 {
		t.Fatalf("unexpected header read: %#v", reads[0])
	}
	if reads[1].offset <= 6*1024*1024*1024 || reads[1].length > 256*1024 {
		t.Fatalf("far cache read was not bounded/random-access: %#v", reads[1])
	}
	file.mu.Lock()
	maxRead := file.maxRead
	file.mu.Unlock()
	if maxRead > 256*1024 {
		t.Fatalf("largest read=%d, want at most 256 KiB", maxRead)
	}
}

func TestSemantic_ViewerNoWrapWindowConsumesWholeLogicalLine(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "long-line.txt")
	first := strings.Repeat("a", 300)
	content := first + "\nsecond\nthird\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.WrapMode = false
	viewer.SetPosition(0, 0, 19, 5)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	rows := semantic.AppMapSlice(awaitSemanticViewerWindow(t, viewer, 2)["windowRows"])
	if len(rows) < 2 {
		t.Fatalf("rows = %#v", rows)
	}
	if got, want := semantic.AppInt64(rows[1]["offset"]), int64(len(first)+1); got != want {
		t.Fatalf("second logical row offset=%d, want %d", got, want)
	}
}

func TestSemantic_ViewerWrappedScrollWindowUsesVisualRows(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "wrapped-window.txt")
	content := strings.Repeat("x", 64*1024) + "\nnext\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.SetPosition(0, 0, 19, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	width := viewer.semanticContentWidth()
	if width != 19 {
		t.Fatalf("viewer content width=%d, want 19", width)
	}
	// NewViewerView deliberately seeds the first 16 KiB encoding-probe prefix
	// into the bounded backend cache. Seek beyond it to exercise async loading.
	targetRow := 2048
	wantTop := int64(targetRow * width)
	requestedOffset := wantTop + 7
	beforeGeneration := viewer.semanticWindowGeneration
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": requestedOffset,
	}) {
		t.Fatal("wrapped viewer scroll was not handled")
	}
	if !viewer.SemanticPendingScroll {
		t.Fatal("initial uncached wrapped seek did not enter pending state")
	}

	deadline := time.After(2 * time.Second)
	node := viewer.SemanticNode(nil)
	for viewer.SemanticPendingScroll {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timed out resolving wrapped viewer seek: top=%d", viewer.TopOffset)
		}
		node = viewer.SemanticNode(nil)
	}
	if viewer.TopOffset != wantTop {
		t.Fatalf("wrapped seek top=%d, want visual row start %d", viewer.TopOffset, wantTop)
	}
	if viewer.semanticWindowGeneration != beforeGeneration+1 {
		t.Fatalf("wrapped seek generation=%d, want %d",
			viewer.semanticWindowGeneration, beforeGeneration+1)
	}
	if node == nil {
		node = viewer.SemanticNode(nil)
	}

	windowRows := semantic.AppMapSlice(node["windowRows"])
	viewportRow := semantic.Int(node["viewportRow"])
	if viewportRow < 0 || viewportRow >= len(windowRows) {
		t.Fatalf("viewportRow=%d outside %d wrapped rows", viewportRow, len(windowRows))
	}
	if got := semantic.AppInt64(windowRows[viewportRow]["offset"]); got != wantTop {
		t.Fatalf("wrapped viewport row offset=%d, want %d", got, wantTop)
	}
	bufferRows := semantic.SemanticWindowBufferRows(8)
	wantWindowStart := wantTop - int64(bufferRows*width)
	if got := semantic.AppInt64(node["windowStart"]); got != wantWindowStart {
		t.Fatalf("wrapped window start=%d, want %d visual rows before viewport",
			got, wantWindowStart)
	}

	// The same resolver must also work synchronously once the bounded backend
	// cache is warm; otherwise only the delayed/pending path would be covered.
	nextRow := targetRow + 7
	nextTop := int64(nextRow * width)
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": nextTop + 3,
	}) {
		t.Fatal("cached wrapped viewer scroll was not handled")
	}
	if !viewer.SemanticPendingScroll || viewer.TopOffset != wantTop {
		t.Fatal("cached request committed before the requested projection")
	}
	node = viewer.SemanticNode(nil)
	if viewer.SemanticPendingScroll {
		t.Fatal("cached wrapped projection did not acknowledge its ready window")
	}
	if viewer.TopOffset != nextTop {
		t.Fatalf("cached wrapped seek top=%d, want visual row start %d",
			viewer.TopOffset, nextTop)
	}
}

func TestSemantic_ViewerWrappedSeekResumesAcrossCacheWindows(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "megabyte-wrapped-window.txt")
	content := strings.Repeat("x", 1280*1024) // Deliberately no newline.
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	viewer, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmp), path)
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	// Exercise asynchronous cache-window negotiation explicitly. OS readers
	// now use bounded direct-local reads and need not take the pending path.
	viewer.Backend.File = semanticExpensiveReader{viewer.Backend.File}
	viewer.SetPosition(0, 0, 19, 8)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	width := viewer.semanticContentWidth()
	targetRow := (1024 * 1024) / width
	wantTop := int64(targetRow * width)
	requestedOffset := wantTop + int64(width/2)
	if !viewer.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(viewer),
		"action": "viewer.scrollWindow",
		"offset": requestedOffset,
	}) {
		t.Fatal("megabyte wrapped viewer scroll was not handled")
	}
	if !viewer.SemanticPendingScroll {
		t.Fatal("megabyte wrapped seek unexpectedly resolved without cache fills")
	}

	deadline := time.After(10 * time.Second)
	cacheFills := 0
	node := viewer.SemanticNode(nil) // Projection initiates readiness-driven I/O.
	for viewer.SemanticPendingScroll {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			cacheFills++
		case <-deadline:
			t.Fatalf("wrapped seek did not resume to %d: curr=%d target=%d fills=%d",
				wantTop, viewer.SemanticWrapSeek.curr,
				viewer.SemanticWrapSeek.target, cacheFills)
		}
		node = viewer.SemanticNode(nil)
	}
	if cacheFills < 4 {
		t.Fatalf("test crossed only %d cache fills; expected a multi-window seek", cacheFills)
	}
	if viewer.TopOffset != wantTop {
		t.Fatalf("megabyte wrapped seek top=%d, want %d", viewer.TopOffset, wantTop)
	}

	bufferRows := semantic.SemanticWindowBufferRows(8)
	wantHistory := bufferRows + 1
	seek := &viewer.SemanticWrapSeek
	if seek.active || !seek.ready {
		t.Fatalf("completed wrapped seek state active=%v ready=%v", seek.active, seek.ready)
	}
	if len(seek.history) != wantHistory || seek.historyCount != wantHistory {
		t.Fatalf("wrapped history backing=%d count=%d, want bounded %d",
			len(seek.history), seek.historyCount, wantHistory)
	}
	previous := wantTop
	for row := 1; row <= bufferRows; row++ {
		got, ok := seek.previousHistoryOffset(previous, width)
		want := wantTop - int64(row*width)
		if !ok || got != want {
			t.Fatalf("history predecessor %d=(%d,%v), want %d", row, got, ok, want)
		}
		previous = got
	}
	viewer.Backend.mu.Lock()
	cachedBytes := len(viewer.Backend.cacheData)
	viewer.Backend.mu.Unlock()
	if cachedBytes > 256*1024 {
		t.Fatalf("wrapped seek retained %d cache bytes, want at most 256 KiB", cachedBytes)
	}

	if node == nil {
		node = viewer.SemanticNode(nil)
	}
	wantWindowStart := wantTop - int64(bufferRows*width)
	if got := semantic.AppInt64(node["windowStart"]); got != wantWindowStart {
		t.Fatalf("megabyte wrapped window start=%d, want %d", got, wantWindowStart)
	}
	viewportRow := semantic.Int(node["viewportRow"])
	rows := semantic.AppMapSlice(node["windowRows"])
	if viewportRow != bufferRows || viewportRow >= len(rows) ||
		semantic.AppInt64(rows[viewportRow]["offset"]) != wantTop {
		t.Fatalf("megabyte wrapped viewport row=%d rows=%#v", viewportRow, rows)
	}
}
