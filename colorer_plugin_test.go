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
	for i := 0; i < maxCachedAttrLines+64; i++ {
		ch.storeAttrs(i, []uint64{uint64(i)}, 0)
	}
	if len(ch.attrCache) > maxCachedAttrLines {
		t.Errorf("Attribute cache grew to %d entries", len(ch.attrCache))
	}
	if _, ok := ch.attrCache[maxCachedAttrLines+63]; !ok {
		t.Error("Expected the most recent line to stay cached")
	}

	ch.dropCacheFrom(0)
	if len(ch.attrCache) != 0 {
		t.Errorf("Expected an empty cache after invalidation, got %d entries", len(ch.attrCache))
	}
}

func TestColorer_LineIndexComesFromTheEditorState(t *testing.T) {
	cases := []struct {
		prevState any
		known     int
		want      int
	}{
		{nil, 0, 0},
		{nil, 500, 0},
		{7, 500, 8},
		{"a state of the fallback engine", 500, 0},
		{499, 500, 500},
		{500, 500, 500},
		{-9, 500, 0},
	}
	for _, c := range cases {
		if got := colorerLineIndex(c.prevState, c.known); got != c.want {
			t.Errorf("colorerLineIndex(%v, %d) = %d, expected %d", c.prevState, c.known, got, c.want)
		}
	}
}

func TestColorer_RewindsOnlyWhenTheParserIsAhead(t *testing.T) {
	if colorerNeedsRewind(3, 7) {
		t.Error("Expected the parser to be fed forward while it is behind")
	}
	if colorerNeedsRewind(7, 7) {
		t.Error("Expected no work when the parser is already in place")
	}
	if !colorerNeedsRewind(9, 7) {
		t.Error("Expected a rewind when the parser is past the requested line")
	}
	if !colorerNeedsRewind(-1, 7) {
		t.Error("Expected a rewind from an unknown parser position")
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
