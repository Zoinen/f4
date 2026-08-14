package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/piecetable"

	"github.com/unxed/vtui"
)

func TestColorer_EnsureFonokaiSchema(t *testing.T) {
	tmpDir := t.TempDir()

	baseDir := filepath.Join(tmpDir, "base")
	_ = os.MkdirAll(baseDir, 0755)
	_ = os.WriteFile(filepath.Join(baseDir, "catalog.xml"), []byte("<catalog/>"), 0644)

	hrdDir := filepath.Join(baseDir, "hrd")
	_ = os.MkdirAll(hrdDir, 0755)

	catalogRGBContent := `        <hrd class="rgb" name="default" description="far2l default">
            <location link="&hrd;/rgb/default.hrd"/>
        </hrd>`
	_ = os.WriteFile(filepath.Join(hrdDir, "catalog-rgb.xml"), []byte(catalogRGBContent), 0644)

	ensureFonokaiSchema(tmpDir)

	hrdFile := filepath.Join(hrdDir, "rgb", "fonokai.hrd")
	if _, err := os.Stat(hrdFile); err != nil {
		t.Errorf("Expected fonokai.hrd to be created, got error: %v", err)
	}

	updatedCatalog, err := os.ReadFile(filepath.Join(hrdDir, "catalog-rgb.xml"))
	if err != nil {
		t.Fatalf("Failed to read updated catalog: %v", err)
	}
	if !strings.Contains(string(updatedCatalog), "name=\"Fonokai\"") {
		t.Error("Expected catalog-rgb.xml to contain Fonokai entry")
	}
}
func TestColorer_FonokaiHRDContrastAndSync(t *testing.T) {
	hrdDiskPath := filepath.Join("colorer", "configs", "base", "hrd", "rgb", "fonokai.hrd")
	diskBytes, err := os.ReadFile(hrdDiskPath)
	if err != nil {
		t.Fatalf("Failed to read fonokai.hrd from disk: %v", err)
	}

	diskContent := string(diskBytes)
	if strings.Contains(diskContent, "#a04020") || strings.Contains(diskContent, "fore=\"#cc5f30\"") {
		t.Error("fonokai.hrd on disk contains low-contrast keyword/tag colors (#a04020 or #cc5f30)")
	}

	if !strings.Contains(diskContent, "name=\"def:Keyword\" fore=\"#ff5544\"") {
		t.Error("fonokai.hrd on disk does not use high-contrast color #ff5544 for def:Keyword")
	}

	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "base")
	_ = os.MkdirAll(baseDir, 0755)
	_ = os.WriteFile(filepath.Join(baseDir, "catalog.xml"), []byte("<catalog/>"), 0644)
	_ = os.WriteFile(filepath.Join(baseDir, "hrd", "catalog-rgb.xml"), []byte("<hrd/>"), 0644)

	ensureFonokaiSchema(tmpDir)

	generatedFile := filepath.Join(tmpDir, "base", "hrd", "rgb", "fonokai.hrd")
	genBytes, err := os.ReadFile(generatedFile)
	if err != nil {
		t.Fatalf("ensureFonokaiSchema failed to write fonokai.hrd: %v", err)
	}

	if string(genBytes) != diskContent {
		t.Errorf("fonokaiHRDContent in colorer_plugin.go does not match fonokai.hrd on disk")
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
	_, _ = f.Write([]byte("<catalog></catalog>"))

	zw.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(buf.Bytes())
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
