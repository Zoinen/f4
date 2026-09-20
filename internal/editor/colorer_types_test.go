package editor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/vtui"
)

func TestColorerTypeRowAndHotkey(t *testing.T) {
	if got := colorerTypeRow(colorerTypeEntry{description: "C & C++", hotkey: "C"}); got != "&C C && C++" {
		t.Errorf("row %q", got)
	}
	if got := colorerTypeRow(colorerTypeEntry{description: "JSON"}); got != "  JSON" {
		t.Errorf("row %q", got)
	}
	for in, want := range map[string]string{"j": "J", " 7x": "7", "-": "", "": "", "ж": "Ж"} {
		if got := colorerHotkey(in); got != want {
			t.Errorf("colorerHotkey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestColorerRegionAt(t *testing.T) {
	spans := []colorerRegionSpan{{0, -1}, {4, 9}, {5, 8}}
	if got, ok := colorerRegionAt(spans, 6, 12); !ok || got != (colorerRegionSpan{5, 8}) {
		t.Errorf("inner = %+v, %v", got, ok)
	}
	if got, ok := colorerRegionAt(spans, 9, 12); !ok || got != (colorerRegionSpan{4, 9}) {
		t.Errorf("at the end of a region = %+v, %v", got, ok)
	}
	if got, ok := colorerRegionAt(spans, 11, 12); !ok || got != (colorerRegionSpan{0, 12}) {
		t.Errorf("line-wide = %+v, %v", got, ok)
	}
}

func TestColorerProfileRoundTrip(t *testing.T) {
	config.GetF4ConfigDir() // resolve once, so the cache is what is swapped
	old := config.CachedF4ConfigDir
	dir := t.TempDir()
	config.CachedF4ConfigDir = dir
	t.Cleanup(func() { config.CachedF4ConfigDir = old })

	profile := map[string]map[string]string{"json": {"favorite": "true", "hotkey": "J"}, "c": {"favorite": "false"}}
	if err := saveColorerProfile(profile); err != nil {
		t.Fatal(err)
	}
	got := loadColorerProfile()
	if got["json"]["favorite"] != "true" || got["json"]["hotkey"] != "J" || got["c"]["favorite"] != "false" {
		t.Errorf("profile read back as %v", got)
	}
}

// Picking a type from the list rehighlights the file as that type: the
// worker resets its session with SetFileType.
func TestColorerSetFileType_RehighlightsAsTheType(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)

	// The type carries parameters of its own, and the worker is supposed to
	// hand them to the UI along with the type. One is given to pairtest here
	// so that arrival is visible: a parameter nobody set is worth the zero
	// value, which is also what settings that were never adopted are worth.
	settingsPath := filepath.Join(t.TempDir(), "hrcsettings.xml")
	if err := os.WriteFile(settingsPath, []byte(`<hrc-settings><prototype name="pairtest"><param name="maxlinelength" value="80"/></prototype></hrc-settings>`), 0o600); err != nil {
		t.Fatal(err)
	}

	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: user, UserHRCSettings: settingsPath}

	session, err := acquireCancelableColorerSession(context.Background(), src)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if err := session.SetHRD("rgb", "default"); err != nil {
		t.Fatalf("SetHRD: %v", err)
	}
	// "a.txt" matches no type of this catalog; the parser gets none of the
	// pair test's regions.
	session.SelectType("a.txt", "")

	ev := NewEditorView(piecetable.New([]byte("{\n}\n")), nil, "a.txt")
	defer ev.Close()
	ch := &ColorerHighlighter{owner: ev, postTask: vtui.FrameManager.PostTask, colorerSrc: src, filename: "a.txt"}
	// A redraw asks for the line again, as the editor's drawing does: new
	// type settings drop what was computed with the old ones.
	ch.redraw = func() { ch.HighlightLine(0, "{", 0) }
	ch.SetLineSource(ev.lineTextForHighlight)
	ev.Highlighter = ch
	ch.session = session
	ch.startWorker(session)

	ch.HighlightLine(0, "{", 0)
	pumpUntil(t, "line 0 parsed", func() bool { _, ok := ch.cachedPairs(0); return ok && !ch.pending })
	if pairs, _ := ch.cachedPairs(0); len(pairs) != 0 {
		t.Fatalf("pairs before choosing the type: %+v", pairs)
	}

	ch.setFileType("pairtest")
	ch.HighlightLine(0, "{", 0)
	pumpUntil(t, "line 0 parsed as pairtest", func() bool { _, ok := ch.cachedPairs(0); return ok && !ch.pending })
	if pairs, _ := ch.cachedPairs(0); len(pairs) != 1 || !pairs[0].Opens {
		t.Fatalf("pairs as pairtest: %+v", pairs)
	}
	if ch.typeSettings.maxLineLength != 80 {
		t.Errorf("type settings %+v were not adopted from the worker", ch.typeSettings)
	}

}

// The quick view's colorizer highlights a text from its first line with the
// editor's highlighter and hands the colours to the UI thread; it is off
// unless the viewer highlighting setting asks for it.
func TestNewTextColorizer_ColoursTheQuickView(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	saved := config.App
	t.Cleanup(func() { config.App = saved; ResetColorerSessions() })
	config.App.EditorHighlighter = "Colorer"
	config.App.EditorColorerSyntax = true
	config.App.EditorColorerCatalog = checkConfigs(t)
	config.App.EditorColorerUserHrc = user

	config.App.ViewerHighlighting = config.ViewerHighlightOff
	if c := NewTextColorizer("a.pairtest", []string{"fn alpha"}, 7, true, nil); c != nil {
		t.Fatal("highlighted with viewer highlighting off")
	}
	config.App.ViewerHighlighting = config.ViewerHighlightQuickView
	if c := NewTextColorizer("a.pairtest", []string{"fn alpha"}, 7, false, nil); c != nil {
		t.Fatal("highlighted a viewer in the quick view only mode")
	}

	// Chroma registers itself from its plugin at run time; a stand-in
	// registered for its own extension takes its place here.
	vtui.RegisterHighlighter(textColorizerTestProvider{})
	for _, engine := range []struct{ highlighter, path string }{{"Colorer", "a.pairtest"}, {"Chroma", "a.textcolorizertest"}} {
		config.App.EditorHighlighter = engine.highlighter
		lines := []string{"package main", "", "x Ж"}
		c := NewTextColorizer(engine.path, lines, 7, true, nil)
		if c == nil {
			t.Fatalf("%s: no colorizer for the quick view", engine.highlighter)
		}
		pumpUntil(t, engine.highlighter+" colours", func() bool { return c.LineAttrs(2) != nil })
		if got := c.LineAttrs(0); len(got) != len([]rune(lines[0])) {
			t.Errorf("%s: line 0 has %d attributes, want one per rune", engine.highlighter, len(got))
		}
		c.Close()
	}
	config.App.EditorHighlighter = "None"
	if c := NewTextColorizer("a.textcolorizertest", []string{"package main"}, 7, true, nil); c != nil {
		t.Error("highlighted with the highlighter set to None")
	}
}

// textColorizerTestProvider stands in for the Chroma plugin: one attribute per
// rune, and the line number carried as the state.
type textColorizerTestProvider struct{}

func (textColorizerTestProvider) Name() string { return "text colorizer test" }

func (textColorizerTestProvider) Match(filename, content string) bool {
	return strings.HasSuffix(filename, ".textcolorizertest")
}

func (textColorizerTestProvider) Create(filename, content string) vtui.Highlighter {
	return textColorizerTestHighlighter{}
}

type textColorizerTestHighlighter struct{}

func (textColorizerTestHighlighter) Highlight(line string, prevState any, baseAttr uint64) ([]uint64, any) {
	n, _ := prevState.(int)
	attrs := make([]uint64, len([]rune(line)))
	for i := range attrs {
		attrs[i] = baseAttr + uint64(n)
	}
	return attrs, n + 1
}

// A viewer window is highlighted from its context lines on, with the editor's
// highlighter, and its colours are kept by line offset and text.
func TestNewWindowColorizer(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	saved := config.App
	t.Cleanup(func() { config.App = saved; ResetColorerSessions() })
	config.App.EditorColorerSyntax = true
	config.App.EditorColorerCatalog = checkConfigs(t)
	config.App.EditorColorerUserHrc = user
	vtui.RegisterHighlighter(textColorizerTestProvider{})

	config.App.EditorHighlighter = "Chroma"
	config.App.ViewerHighlighting = config.ViewerHighlightQuickView
	if w := NewWindowColorizer("a.textcolorizertest", "", 7, nil); w != nil {
		t.Fatal("a viewer highlighted in the quick view only mode")
	}
	config.App.ViewerHighlighting = config.ViewerHighlightAll

	w := NewWindowColorizer("a.textcolorizertest", "", 7, nil)
	if w == nil {
		t.Fatal("no colorizer for a viewer")
	}
	w.Request([]string{"above 1", "above 2"}, []viewer.WindowLine{{Offset: 16, Text: "abc"}, {Offset: 20, Text: ""}})
	pumpUntil(t, "the window", func() bool { return w.LineAttrs(16, "abc") != nil })
	// The stand-in counts lines in its state: two context lines came first.
	if got := w.LineAttrs(16, "abc"); len(got) != 3 || got[0] != 7+2 {
		t.Errorf("attrs %v, want three runes coloured after two context lines", got)
	}
	if w.LineAttrs(16, "changed") != nil {
		t.Error("colours returned for a line whose text changed")
	}
	w.Close()

	config.App.EditorHighlighter = "Colorer"
	w = NewWindowColorizer("a.pairtest", "fn alpha", 7, nil)
	if w == nil {
		t.Fatal("no Colorer colorizer for a viewer")
	}
	defer w.Close()
	w.Request(nil, []viewer.WindowLine{{Offset: 0, Text: "fn alpha"}})
	pumpUntil(t, "the Colorer window", func() bool { return w.LineAttrs(0, "fn alpha") != nil })
	if got := w.LineAttrs(0, "fn alpha"); len(got) != 8 {
		t.Errorf("Colorer attrs %d, want 8", len(got))
	}
}

// Reloading the Colorer base restarts Colorer in an open editor the next time
// it is drawn, forgetting the type picked for it; updating the highlighting
// drops the colours computed.
func TestColorerReloadAndUpdate(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	saved := config.App
	t.Cleanup(func() { config.App = saved; ResetColorerSessions() })
	config.App.EditorHighlighter = "Colorer"
	config.App.EditorColorerSyntax = true
	config.App.EditorColorerCatalog = checkConfigs(t)
	config.App.EditorColorerUserHrc = user

	ev := NewEditorView(piecetable.New([]byte("{\n}\n")), nil, "a.pairtest")
	defer ev.Close()
	first, ok := ev.Highlighter.(*ColorerHighlighter)
	if !ok {
		t.Fatalf("highlighter %T, want Colorer", ev.Highlighter)
	}
	first.fileTypeOverride = "c"
	first.attrCache = map[int][]uint64{0: {1}}
	ev.ColorerUpdateHighlighting()
	if len(first.attrCache) != 0 {
		t.Errorf("update left %d cached lines", len(first.attrCache))
	}

	ev.restartColorerAfterReload()
	if ev.Highlighter != vtui.Highlighter(first) {
		t.Fatal("restarted without a reload")
	}
	ReloadColorerEditors()
	ev.restartColorerAfterReload()
	second, ok := ev.Highlighter.(*ColorerHighlighter)
	if !ok || second == first {
		t.Fatalf("highlighter %T after a reload, want a new Colorer", ev.Highlighter)
	}
	if !first.closed {
		t.Error("the old highlighter was not closed")
	}
	if second.fileTypeOverride != "" || second.filename != "a.pairtest" {
		t.Errorf("new highlighter: type %q, file %q", second.fileTypeOverride, second.filename)
	}

	// Colorer switched off meanwhile: the editor goes to the other engine.
	config.App.EditorHighlighter = "Chroma"
	ReloadColorerEditors()
	ev.restartColorerAfterReload()
	if _, still := ev.Highlighter.(*ColorerHighlighter); still {
		t.Error("Colorer restarted though it is no longer the highlighter")
	}
}
