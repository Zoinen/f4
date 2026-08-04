package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestColorer_GetColorerAttr(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	base := uint64(0)
	attr := getColorerAttr("def:comment", base)
	if vtui.GetRGBFore(attr) != ColorerColorMap["def:comment"] {
		t.Errorf("Expected comment color, got %06X", vtui.GetRGBFore(attr))
	}

	attrSub := getColorerAttr("def:comment_nested", base)
	if vtui.GetRGBFore(attrSub) != ColorerColorMap["def:comment"] {
		t.Errorf("Expected inherited comment color, got %06X", vtui.GetRGBFore(attrSub))
	}

	attrNone := getColorerAttr("unknown_region", base)
	if attrNone != base {
		t.Error("Expected base attribute for unknown region")
	}
}

func TestColorer_SchemasExist(t *testing.T) {
	_ = SchemasExist()
}

func TestColorer_RegionResolution(t *testing.T) {
	base := uint64(0)
	cases := map[string]uint32{
		"smarty:OpenTag":      ColorerColorMap["tag"],
		"html:htmlOpenTag":    ColorerColorMap["tag"],
		"def:PairStart":       ColorerColorMap["def:pair"],
		"def:Var":             ColorerColorMap["def:var"],
		"def:StringContent":   ColorerColorMap["def:string"],
		"html:Header1Outline": ColorerColorMap["outline"],
	}
	for name, want := range cases {
		got := vtui.GetRGBFore(getColorerAttr(name, base))
		if got != want {
			t.Errorf("Region %q resolved to %06X, expected %06X", name, got, want)
		}
	}
}

func TestColorer_UTF16ToRuneIndex(t *testing.T) {
	cases := []struct {
		line string
		want []int
	}{
		{"", []int{0}},
		{"abc", []int{0, 1, 2, 3}},
		{"aé b", []int{0, 1, 2, 3, 4}},
		{"a\U0001F600b", []int{0, 1, 1, 2, 3}},
	}
	for _, c := range cases {
		got := colorerUTF16ToRuneIndex(c.line)
		if len(got) != len(c.want) {
			t.Errorf("Line %q mapped to %v, expected %v", c.line, got, c.want)
			continue
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Errorf("Line %q mapped to %v, expected %v", c.line, got, c.want)
				break
			}
		}
	}
}

func TestColorer_AttrCacheIsBounded(t *testing.T) {
	// placeholder
	ch := &ColorerHighlighter{}
	for i := 0; i < maxCachedAttrLines+64; i++ {
		ch.storeAttrs(i, []uint64{uint64(i)})
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
		// A redraw asks for line zero again while the parser has already seen
		// the whole file. It must stay line zero instead of landing at the end
		// of the array and dragging a copy of the line along with it.
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

	// Create a mock zip in memory
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Create expected folder structure inside zip: far2l-v_2.8.0/colorer/configs/base/catalog.xml
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

	// Redirect GetF4ConfigDir to temporary location during test
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

	// Process event loop tasks
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

	// Verify extracted files exist
	if !SchemasExist() {
		t.Error("SchemasExist returned false after successful extraction")
	}
}
