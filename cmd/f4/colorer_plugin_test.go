package main

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	embedded "github.com/unxed/f4"
	"github.com/unxed/f4/piecetable"

	"github.com/unxed/vtui"
)

func TestColorer_EnsureRadiolaSchema(t *testing.T) {
	tmpDir := t.TempDir()

	baseDir := filepath.Join(tmpDir, "base")
	_ = os.MkdirAll(baseDir, 0700)
	_ = os.WriteFile(filepath.Join(baseDir, "catalog.xml"), []byte("<catalog/>"), 0600)

	hrdDir := filepath.Join(baseDir, "hrd")
	_ = os.MkdirAll(hrdDir, 0700)

	catalogRGBContent := `        <hrd class="rgb" name="default" description="far2l default">
            <location link="&hrd;/rgb/default.hrd"/>
        </hrd>`
	_ = os.WriteFile(filepath.Join(hrdDir, "catalog-rgb.xml"), []byte(catalogRGBContent), 0600)

	ensureRadiolaSchema(tmpDir)

	hrdFile := filepath.Join(hrdDir, "rgb", "radiola.hrd")
	if _, err := os.Stat(hrdFile); err != nil {
		t.Errorf("Expected radiola.hrd to be created, got error: %v", err)
	}

	updatedCatalog, err := os.ReadFile(filepath.Join(hrdDir, "catalog-rgb.xml"))
	if err != nil {
		t.Fatalf("Failed to read updated catalog: %v", err)
	}
	if !strings.Contains(string(updatedCatalog), "name=\"Radiola\"") {
		t.Error("Expected catalog-rgb.xml to contain Radiola entry")
	}

	generated, err := os.ReadFile(hrdFile)
	if err != nil {
		t.Fatalf("Failed to read generated radiola.hrd: %v", err)
	}
	if string(generated) != embedded.RadiolaHRD {
		t.Error("generated Radiola HRD differs from the repository's canonical embedded schema")
	}
}

func TestColorer_SchemasExist(t *testing.T) {
	_ = SchemasExist()
}

func TestColorer_RegionOffsetsAreRuneIndices(t *testing.T) {
	// Colorer counts code points, so the last character of this line is rune
	// 2 of 3. The old code read the offsets as UTF-16 units and mapped them
	// through a surrogate aware table, which turned the pair (2, 3) into
	// (1, 2): every colour after the emoji landed one position to the left
	// and stayed there to the end of the line.
	const line = "a\U0001F600b"

	if got := colorerLineRuneCount(line); got != 3 {
		t.Fatalf("Expected 3 attributes for %q, got %d", line, got)
	}
	if got := colorerLineRuneCount(line); got != utf8.RuneCountInString(line) {
		t.Errorf("Expected the attribute count to follow the rune count, got %d", got)
	}

	cases := []struct {
		start, end, lineRunes int
		wantStart, wantEnd    int
		wantEOL               bool
	}{
		{2, 3, 3, 2, 3, false},
		{0, 1, 3, 0, 1, false},
		{1, -1, 3, 1, 3, true},
		{0, 9, 3, 0, 3, false},
		{-4, 2, 3, 0, 2, false},
		{5, 9, 3, 3, 3, false},
		{2, 1, 3, 2, 2, false},
	}
	for _, c := range cases {
		start, end, eol := colorerRegionRunes(c.start, c.end, c.lineRunes)
		if start != c.wantStart || end != c.wantEnd || eol != c.wantEOL {
			t.Errorf("colorerRegionRunes(%d, %d, %d) = (%d, %d, %v), expected (%d, %d, %v)",
				c.start, c.end, c.lineRunes,
				start, end, eol,
				c.wantStart, c.wantEnd, c.wantEOL)
		}
	}
}

func TestColorer_AttributesLandOnTheCellsAfterAnEmoji(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	// Runes: 'a', the emoji, 'b', 'c'.
	// Cells: 'a', the emoji, its filler, 'b', 'c'.
	text := []byte("a\U0001F600bc")
	pt := piecetable.New(text)
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()

	const hot = uint64(0x40)
	syntax := []uint64{0, 0, hot, hot}

	cells := ev.fillCells(nil, text, 0, 0, 0, false, 0, 0, syntax, 0, false, -1, 0, 0, 0)
	if len(cells) != 5 {
		t.Fatalf("Expected 5 cells for %q, got %d", text, len(cells))
	}
	for i, want := range []uint64{0, 0, 0, hot, hot} {
		if cells[i].Attributes != want {
			t.Errorf("Cell %d carries attribute %#x, expected %#x", i, cells[i].Attributes, want)
		}
	}
}

func TestColorer_AttrCacheIsBounded(t *testing.T) {
	ch := &ColorerHighlighter{}
	for i := 0; i < 100; i++ {
		ch.storeAttrs(i, []uint64{uint64(i)}, 0)
	}
	if len(ch.attrCache) != 100 {
		t.Errorf("Attribute cache should have 100 entries, got %d", len(ch.attrCache))
	}

	ch.dropCacheFrom(0)
	if len(ch.attrCache) != 0 {
		t.Errorf("Expected an empty cache after invalidation, got %d entries", len(ch.attrCache))
	}
}

func TestColorerQueueLineBoundsDocumentSnapshot(t *testing.T) {
	ch := &ColorerHighlighter{
		lineAt:     func(idx int) (string, bool) { return "context", idx >= 0 },
		workerJobs: make(chan colorerJob, 1),
	}

	ch.queueLine(4000, "target", 0)
	job := <-ch.workerJobs
	if len(job.context) != hlColorerContext {
		t.Fatalf("jump snapshot contains %d lines, want bounded %d", len(job.context), hlColorerContext)
	}
	if job.contextStart != 4000-hlColorerContext || !job.reset {
		t.Fatalf("jump was not re-anchored: start=%d reset=%v", job.contextStart, job.reset)
	}
}

// One job covers the whole uncoloured run, so a viewport colours in a single
// worker round trip instead of one redraw per line.
func TestColorerQueueLineBatchesTheUncolouredRun(t *testing.T) {
	ch := &ColorerHighlighter{
		lineAt:     func(idx int) (string, bool) { return "text", idx < 50 },
		workerJobs: make(chan colorerJob, 1),
	}

	ch.queueLine(10, "target", 0)
	job := <-ch.workerJobs
	if len(job.lines) != 40 {
		t.Fatalf("batch holds %d lines, want the 40 remaining document lines", len(job.lines))
	}
	if job.lines[0] != "target" {
		t.Errorf("the first batched line must be the caller's snapshot, got %q", job.lines[0])
	}
	if job.total != len(job.context)+len(job.lines) {
		t.Errorf("total = %d, want context %d + batch %d", job.total, len(job.context), len(job.lines))
	}
}

func TestColorerQueueLineBatchStopsAtCachedLines(t *testing.T) {
	ch := &ColorerHighlighter{
		attrCache:  map[int][]uint64{12: {0}},
		lineAt:     func(idx int) (string, bool) { return "text", true },
		workerJobs: make(chan colorerJob, 1),
	}

	ch.queueLine(10, "target", 0)
	job := <-ch.workerJobs
	if len(job.lines) != 2 {
		t.Fatalf("batch holds %d lines, want 2: it must stop at the already-coloured line", len(job.lines))
	}
}

func TestColorerQueueLineBatchIsBounded(t *testing.T) {
	ch := &ColorerHighlighter{
		lineAt:     func(idx int) (string, bool) { return "text", true },
		workerJobs: make(chan colorerJob, 1),
	}

	ch.queueLine(0, "target", 0)
	job := <-ch.workerJobs
	if len(job.lines) != hlColorerBatchLines {
		t.Fatalf("batch holds %d lines, want the hlColorerBatchLines cap of %d", len(job.lines), hlColorerBatchLines)
	}
}

// Lines are cut at 64 KB each, so a line count alone would let one snapshot
// copy ~12.8 MB inside a frame; the byte budget bounds the copy instead.
func TestColorerQueueLineBatchRespectsTheByteBudget(t *testing.T) {
	big := strings.Repeat("x", 64*1024)
	ch := &ColorerHighlighter{
		lineAt:     func(idx int) (string, bool) { return big, true },
		workerJobs: make(chan colorerJob, 1),
	}

	ch.queueLine(0, big, 0)
	job := <-ch.workerJobs
	if want := hlColorerBatchBytes / len(big); len(job.lines) != want {
		t.Fatalf("batch holds %d huge lines, want %d: the byte budget must cut it short", len(job.lines), want)
	}
}

// A parse error on a prefetched line must not throw away the attributes the
// batch already computed, and must not disable highlighting — only the line
// the viewport actually asked for is allowed to be terminal. The session's
// position after the failure is unknown, so the next job must re-anchor.
func TestColorerPartialResultKeepsAttrsAndForcesReanchor(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	pt := piecetable.New([]byte("a\nb\nc\n"))
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()

	ch := &ColorerHighlighter{
		owner:          ev,
		postTask:       func(f func()) { f() },
		redraw:         func() {},
		pending:        true,
		workGeneration: 1,
	}
	ev.highlighter = ch

	job := colorerJob{id: 1, generation: 1, target: 10, lines: []string{"l0", "l1", "l2"}}
	ch.postColorerResult(colorerResult{
		job:     job,
		lines:   []colorerLineAttrs{{attrs: []uint64{1}}, {attrs: []uint64{2}}},
		partial: true,
	})

	if ch.disabled {
		t.Error("a prefetch failure must not disable highlighting")
	}
	if ch.pending {
		t.Error("the finished job must clear pending")
	}
	if !ch.forceReset {
		t.Error("a partial result must force the next job to re-anchor")
	}
	if ch.parsedIdx != 12 {
		t.Errorf("parsedIdx = %d, want target + the 2 salvaged lines = 12", ch.parsedIdx)
	}
	if _, ok := ch.attrCache[11]; !ok {
		t.Error("salvaged attributes were not stored")
	}
	if _, ok := ch.attrCache[12]; ok {
		t.Error("the unparsed line must not get attributes")
	}
}

func TestColorerCancelInvalidatesWork(t *testing.T) {
	ch := &ColorerHighlighter{pending: true, workGeneration: 7}
	ch.Cancel()
	if !ch.disabled || ch.pending || ch.workGeneration != 8 {
		t.Fatalf("cancel did not invalidate the worker: disabled=%v pending=%v generation=%d", ch.disabled, ch.pending, ch.workGeneration)
	}
}

// TestColorer_AttrCacheStaysBoundedOnALongForwardScroll pushes storeAttrs
// well past maxCachedAttrLines — the way holding PgDn through a large file
// does — and checks eviction actually keeps the map small instead of the
// limit being effectively no limit at all (HIGHLIGHT.md item 3).
func TestColorer_AttrCacheStaysBoundedOnALongForwardScroll(t *testing.T) {
	ch := &ColorerHighlighter{}
	const scrolled = maxCachedAttrLines * 3
	for i := 0; i < scrolled; i++ {
		ch.storeAttrs(i, []uint64{uint64(i)}, 0)
	}

	if len(ch.attrCache) >= maxCachedAttrLines {
		t.Errorf("cache holds %d entries after scrolling %d lines, eviction did not bound it",
			len(ch.attrCache), scrolled)
	}
	if len(ch.bgCache) != len(ch.attrCache) {
		t.Errorf("attrCache and bgCache drifted apart: %d vs %d entries", len(ch.attrCache), len(ch.bgCache))
	}

	// The line just drawn, and its near neighbours, must survive: otherwise
	// every single line drawn during the scroll would cost a re-anchor.
	last := scrolled - 1
	if _, ok := ch.attrCache[last]; !ok {
		t.Error("the most recently drawn line fell out of the cache")
	}
	// Ctrl+Home must stay instant regardless of how far the scroll has gone.
	if _, ok := ch.attrCache[0]; !ok {
		t.Error("line 0 was evicted; Ctrl+Home would no longer be instant")
	}
}

type stubHighlighter struct {
	calls int
}

func (s *stubHighlighter) Highlight(line string, prevState any, baseAttr uint64) ([]uint64, any) {
	s.calls++
	return []uint64{baseAttr}, "stub"
}

func TestColorer_FallbackUntilSessionIsReady(t *testing.T) {
	stub := &stubHighlighter{}
	ch := &ColorerHighlighter{fallback: stub}

	attrs, state := ch.Highlight("package main", nil, 0)
	if len(attrs) != 1 || state != "stub" {
		t.Errorf("Expected the fallback result, got %v, %v", attrs, state)
	}
	if stub.calls != 1 {
		t.Errorf("Expected exactly one fallback call, got %d", stub.calls)
	}

	_ = ch.Close()
	if ch.fallback != nil || !ch.closed {
		t.Error("Expected the highlighter to drop the fallback on close")
	}
}

func TestColorer_PlainWhileTheSessionStartsUp(t *testing.T) {
	stub := &stubHighlighter{}
	ch := &ColorerHighlighter{fallback: stub, starting: true}

	attrs, state := ch.Highlight("package main", nil, 0)
	if attrs != nil || state != nil {
		t.Errorf("Expected nothing to be colored while the session starts, got %v, %v", attrs, state)
	}
	if stub.calls != 0 {
		t.Errorf("Expected the fallback engine to stay unused, got %d calls", stub.calls)
	}

	ch.starting = false
	if _, state := ch.Highlight("package main", nil, 0); state != "stub" {
		t.Error("Expected the fallback engine to take over once Colorer gave up")
	}
}

func TestColorer_HighlighterNilSession(t *testing.T) {
	ch := &ColorerHighlighter{
		session:  nil,
		filename: "test.go",
	}
	attrs, state := ch.Highlight("fmt.Println()", nil, 0)
	if attrs != nil || state != nil {
		t.Errorf("Expected nil, nil for nil session, got %v, %v", attrs, state)
	}
	_ = ch.Close()
}
func TestColorer_StoreAttrsPreservesTopLines(t *testing.T) {
	ch := &ColorerHighlighter{
		attrCache: make(map[int][]uint64),
		bgCache:   make(map[int]uint64),
	}

	ch.storeAttrs(0, []uint64{100}, 0)
	if _, ok := ch.attrCache[0]; !ok {
		t.Error("Expected line 0 to be present")
	}
}

func TestColorer_DownloadColorerSchemas(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	f, err := zw.Create("far2l-v_2.8.0/colorer/configs/base/catalog.xml")
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	if _, err := f.Write([]byte("<catalog></catalog>")); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	// Preserve an initialized config cache when this test temporarily swaps
	// the directory below. Otherwise cleanup can restore an empty cache while
	// configDirOnce remains consumed, making later shuffled tests resolve
	// relative paths.
	_ = GetF4ConfigDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		if _, err := w.Write(buf.Bytes()); err != nil {
			t.Errorf("write schema archive response: %v", err)
		}
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	oldConfigDirFunc := getUserConfigIniPath
	getUserConfigIniPath = func() string { return filepath.Join(tmpDir, "settings.ini") }
	origPathsFunc := getConfigIniPaths
	getConfigIniPaths = func() []string { return []string{filepath.Join(tmpDir, "settings.ini")} }
	oldConfigDir := cachedF4ConfigDir
	cachedF4ConfigDir = tmpDir

	defer func() {
		getUserConfigIniPath = oldConfigDirFunc
		getConfigIniPaths = origPathsFunc
		cachedF4ConfigDir = oldConfigDir
	}()

	oldURL := colorerDownloadURL
	colorerDownloadURL = ts.URL
	defer func() { colorerDownloadURL = oldURL }()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	done := make(chan bool)
	DownloadColorerSchemas(pf, func(success bool) {
		if !success {
			t.Error("Expected successful download and extraction")
		}
		close(done)
	})

	timeout := time.After(3 * time.Second)
Loop:
	for {
		select {
		case <-done:
			break Loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for downloader")
		}
	}

	if !SchemasExist() {
		t.Error("SchemasExist returned false after successful extraction")
	}
}

func TestColorer_StagedInstallRejectsTraversalAndPreservesExistingTree(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "colorer", "configs")
	if err := os.MkdirAll(filepath.Join(dest, "base"), 0o700); err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(dest, "base", "catalog.xml")
	if err := os.WriteFile(catalog, []byte("old-catalog"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	member, err := zw.Create("far2l-v_2.8.0/colorer/configs/../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.Write([]byte("must not escape")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := installColorerSchemas(buf.Bytes(), dest, context.Background()); err == nil {
		t.Fatal("traversal archive unexpectedly installed")
	}
	got, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old-catalog" {
		t.Fatalf("existing catalog changed after rejected archive: %q", got)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dest), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("archive member escaped staging directory: %v", err)
	}
}

func TestColorer_ContextPlan(t *testing.T) {
	if start, reset := colorerContextPlan(120, 140); reset || start != 120 {
		t.Errorf("a short step forward must feed the session, got start=%d reset=%v", start, reset)
	}
	if start, reset := colorerContextPlan(7, 7); reset || start != 7 {
		t.Errorf("no move, no work, got start=%d reset=%v", start, reset)
	}
	if start, reset := colorerContextPlan(0, hlColorerForward); reset || start != 0 {
		t.Error("the forward limit itself must still be reached by feeding")
	}
	// A jump of any size costs the same: the anchor, and nothing before it.
	start, reset := colorerContextPlan(0, 500000)
	if !reset {
		t.Error("a jump past the forward limit must re-anchor")
	}
	if start != 500000-hlColorerContext {
		t.Errorf("anchor placed at %d, expected %d lines of context", start, hlColorerContext)
	}
	// Backwards is a re-anchor whatever the distance: the session cannot be
	// rewound, only thrown away.
	if start, reset := colorerContextPlan(200, 199); !reset || start != 0 {
		t.Errorf("a step back near the top must re-anchor from 0, got start=%d reset=%v", start, reset)
	}
	if _, reset := colorerContextPlan(500000, 499000); !reset {
		t.Error("a step back must re-anchor")
	}
	if start, _ := colorerContextPlan(500, 10); start != 0 {
		t.Errorf("the anchor must not go below the first line, got %d", start)
	}
}

func TestColorer_ForgetPlan(t *testing.T) {
	// Fresh session, nothing fed yet past the batch threshold: no call.
	if _, do := colorerForgetPlan(500, 0); do {
		t.Error("under hlColorerForgetEvery lines fed, forgetBehind should not call yet")
	}
	// Right at the threshold, with a keep window that leaves something to
	// drop: it should call, and land exactly hlColorerKeepBehind behind the
	// parse position.
	keepFrom, do := colorerForgetPlan(hlColorerForgetEvery, 0)
	if !do {
		t.Fatal("at the threshold forgetBehind should call")
	}
	if want := hlColorerForgetEvery - hlColorerKeepBehind; keepFrom != want {
		t.Errorf("keepFrom = %d, want %d", keepFrom, want)
	}
	// Called again immediately after: not enough new lines fed since the
	// last cut, so no second wasm call for the same ground.
	if _, do := colorerForgetPlan(hlColorerForgetEvery, keepFrom); do {
		t.Error("forgetBehind should not repeat work it already did")
	}
	// A fresh anchor sets forgottenUpTo to the anchor itself (resetSessionAt
	// does this): nothing to forget until hlColorerForgetEvery more lines
	// are fed from there, even if parsedIdx is a large absolute number.
	if _, do := colorerForgetPlan(500000, 500000); do {
		t.Error("right after a re-anchor there is nothing new to forget")
	}
	if _, do := colorerForgetPlan(500000+hlColorerForgetEvery-1, 500000); do {
		t.Error("one line short of the threshold should still not call")
	}
	if _, do := colorerForgetPlan(500000+hlColorerForgetEvery, 500000); !do {
		t.Error("hlColorerForgetEvery lines after a re-anchor should call")
	}
}

func TestColorer_DropFromForgetsColours(t *testing.T) {
	ch := &ColorerHighlighter{}
	for i := 0; i < 10; i++ {
		ch.storeAttrs(i, []uint64{uint64(i)}, 0)
	}

	ch.DropFrom(4)
	if _, ok := ch.attrCache[4]; ok {
		t.Error("the edited line kept its colours")
	}
	if _, ok := ch.attrCache[9]; ok {
		t.Error("lines below the edit kept their colours")
	}
	if _, ok := ch.attrCache[3]; !ok {
		t.Error("lines above the edit lost theirs for nothing")
	}
}
