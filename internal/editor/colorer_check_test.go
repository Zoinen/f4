package editor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// checkConfigs is a catalog with one colour style, "default", and no file
// types of its own.
func checkConfigs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Registered after TempDir, so it runs before the directory is removed:
	// Colorer sessions still being set up write into it.
	t.Cleanup(colorerSetups.wait)
	write := func(rel, content string) {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("base/catalog.xml", `<?xml version="1.0" encoding="UTF-8"?>
<catalog xmlns="http://colorer.github.io/schema/v1/catalog">
  <hrc-sets/>
  <hrd-sets>
    <hrd class="rgb" name="default" description="Default">
      <location link="hrd/default.hrd"/>
    </hrd>
  </hrd-sets>
</catalog>
`)
	write("base/hrd/default.hrd", `<hrd xmlns="http://colorer.sf.net/2003/hrd"/>`)
	return dir
}

func writeUserHRC(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

const checkUserType = `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc">
  <prototype name="checktest" group="user" description="Check test">
    <location link="checktest.hrc"/>
    <filename>/\.checktest$/</filename>
  </prototype>
  <type name="checktest">
    <scheme name="checktest"/>
  </type>
</hrc>
`

func TestCheckColorerSource_LoadsEveryType(t *testing.T) {
	user := t.TempDir()
	writeUserHRC(t, user, "checktest.hrc", checkUserType)
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: user}

	var labels []string
	check := CheckColorerSource(context.Background(), src, "", true, func(done, total int, label string) {
		labels = append(labels, label)
	})
	if !check.Clean() {
		t.Fatalf("check = %+v, want a clean one", check)
	}
	if check.Types != 1 || len(labels) != 1 || labels[0] != "user: Check test" {
		t.Errorf("types = %d, progress = %q; want the user's one type", check.Types, labels)
	}
}

// Issue #277: a user scheme Colorer cannot load has to be reported with its
// file, before an editor quietly falls back to another highlighter.
func TestCheckColorerSource_BrokenUserSchemeFails(t *testing.T) {
	// The prototype parses; the type it points to does not. Colorer reads the
	// type only when it is loaded, so only the full check can find it.
	user := t.TempDir()
	writeUserHRC(t, user, "proto.hrc", `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc">
  <prototype name="checktest" group="user" description="Check test">
    <location link="checktest-type.hrc"/>
    <filename>/\.checktest$/</filename>
  </prototype>
</hrc>
`)
	writeUserHRC(t, user, "checktest-type.hrc", `<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc"><type name="checktest">`)
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: filepath.Join(user, "proto.hrc")}

	if quick := CheckColorerSource(context.Background(), src, "", false, nil); quick.Err != nil {
		t.Fatalf("the quick check loads no type schemes, yet failed: %v", quick.Err)
	}
	check := CheckColorerSource(context.Background(), src, "", true, nil)
	if check.Err == nil || !strings.Contains(check.Err.Error(), "checktest") {
		t.Fatalf("Err = %v, want the broken type named", check.Err)
	}
	if !strings.Contains(strings.Join(check.Reports, "\n"), "checktest-type.hrc") {
		t.Errorf("reports %q do not name the broken file", check.Reports)
	}
}

func TestCheckColorerSource_ReportsWhatDidNotStopIt(t *testing.T) {
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRD: filepath.Join(t.TempDir(), "missing")}
	check := CheckColorerSource(context.Background(), src, "", false, nil)
	if check.Err != nil {
		t.Fatalf("a missing user path failed the check: %v", check.Err)
	}
	if !strings.Contains(strings.Join(check.Reports, "\n"), "user colour styles not loaded") {
		t.Errorf("reports %q do not mention the skipped path", check.Reports)
	}
}

func TestCheckColorerSource_UnknownStyleFails(t *testing.T) {
	check := CheckColorerSource(context.Background(), ColorerSource{ConfigsDir: checkConfigs(t)}, "no-such-style", false, nil)
	if check.Err == nil || !strings.Contains(check.Err.Error(), "no-such-style") {
		t.Errorf("Err = %v, want the unknown colour style named", check.Err)
	}
}

// pairTestHRC is a user scheme whose braces are pairs, with the few def
// regions pair matching needs, so a test does not depend on a full catalog.
const pairTestHRC = `<?xml version="1.0" encoding="UTF-8"?>
<hrc version="take5" xmlns="http://colorer.sf.net/2003/hrc">
  <prototype name="def" group="user" description="def">
    <location link="pairtest.hrc"/>
  </prototype>
  <prototype name="pairtest" group="user" description="Pair test">
    <location link="pairtest.hrc"/>
    <filename>/\.pairtest$/</filename>
  </prototype>
  <type name="def">
    <region name="Special"/>
    <region name="PairStart" parent="Special"/>
    <region name="PairEnd" parent="Special"/>
    <region name="Outlined" parent="Special"/>
    <region name="Function" parent="Outlined"/>
    <region name="Error"/>
    <scheme name="def"/>
  </type>
  <type name="pairtest">
    <import type="def"/>
    <scheme name="pairtest">
      <regexp match="/^fn\s+(\w+)/" region1="def:Function"/>
      <regexp match="/\?\?\?/" region="def:Error"/>
      <block start="/(\{)/" end="/(\})/" scheme="pairtest" region00="def:PairStart" region10="def:PairEnd"/>
    </scheme>
  </type>
</hrc>
`

// Issue #277: match pair and select block reach a brace thousands of lines
// away, below and above, through lines the cache has not parsed yet.
func TestColorerPair_WholeFileSearch(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: user}

	session, err := acquireCancelableColorerSession(context.Background(), src)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if err := session.SetHRD("rgb", "default"); err != nil {
		t.Fatalf("SetHRD: %v", err)
	}
	if ok, err := session.SelectType("a.pairtest", ""); err != nil || !ok {
		t.Fatalf("SelectType: %v, %v", ok, err)
	}

	const last = 3001
	text := "{\n" + strings.Repeat("x\n", last-1) + "}\n"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "a.pairtest")
	defer ev.Close()
	// A re-anchoring job selects the type again by name, as the editor does.
	ch := &ColorerHighlighter{owner: ev, postTask: vtui.FrameManager.PostTask, redraw: func() {}, colorerSrc: src, filename: "a.pairtest"}
	ch.SetLineSource(ev.lineTextForHighlight)
	ev.Highlighter = ch
	ch.session = session
	ch.startWorker(session)

	ch.HighlightLine(0, "{", 0)
	pumpUntil(t, "line 0 parsed", func() bool { _, ok := ch.cachedPairs(0); return ok && !ch.pending })
	if pairs, _ := ch.cachedPairs(0); len(pairs) != 1 || !pairs[0].Opens {
		t.Fatalf("line 0 pairs = %+v, want one pair start", pairs)
	}

	ev.CursorLine, ev.CursorPos = 0, 0
	ev.ColorerPair(ColorerMatchPair)
	pumpUntil(t, "match pair below", func() bool { return ch.pairSearch == nil && !ch.pending })
	if ev.CursorLine != last || ev.CursorPos != 0 {
		t.Fatalf("cursor at %d:%d, want %d:0", ev.CursorLine, ev.CursorPos, last)
	}

	// Forget everything but the cursor line, so the walk up has to queue.
	for idx := range ch.attrCache {
		if idx != last {
			delete(ch.attrCache, idx)
			delete(ch.pairCache, idx)
		}
	}
	ev.ColorerPair(ColorerSelectBlock)
	pumpUntil(t, "select block above", func() bool { return ch.pairSearch == nil && !ch.pending })
	end := ev.Li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	if !ev.SelActive || ev.SelAnchorOffset != 0 || end != len(text)-1 {
		t.Fatalf("selection %v from %d to %d, want 0 to %d", ev.SelActive, ev.SelAnchorOffset, end, len(text)-1)
	}
}

// Issue #277: the outline of a whole file, collected through lines the cache
// has not parsed, and locate function over it.
func TestColorerOutline_WholeFileAndLocate(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	user := t.TempDir()
	writeUserHRC(t, user, "pairtest.hrc", pairTestHRC)
	src := ColorerSource{ConfigsDir: checkConfigs(t), UserHRC: user}

	session, err := acquireCancelableColorerSession(context.Background(), src)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if err := session.SetHRD("rgb", "default"); err != nil {
		t.Fatalf("SetHRD: %v", err)
	}
	if ok, err := session.SelectType("a.pairtest", ""); err != nil || !ok {
		t.Fatalf("SelectType: %v, %v", ok, err)
	}

	lines := make([]string, 3000)
	for i := range lines {
		lines[i] = "x"
	}
	lines[10] = "fn alpha"
	lines[20] = "call beta ???"
	lines[2500] = "fn beta"
	text := strings.Join(lines, "\n") + "\n"
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "a.pairtest")
	defer ev.Close()
	ch := &ColorerHighlighter{owner: ev, postTask: vtui.FrameManager.PostTask, redraw: func() {}, colorerSrc: src, filename: "a.pairtest"}
	ch.SetLineSource(ev.lineTextForHighlight)
	ev.Highlighter = ch
	ch.session = session
	ch.startWorker(session)

	var functions, errs []colorerOutlineEntry
	doneF, doneE := false, false
	ch.buildOutline(false, func(e []colorerOutlineEntry) { functions, doneF = e, true })
	pumpUntil(t, "the functions", func() bool { return doneF && !ch.pending })
	ch.buildOutline(true, func(e []colorerOutlineEntry) { errs, doneE = e, true })
	pumpUntil(t, "the errors", func() bool { return doneE && !ch.pending })

	var labels []string
	for _, e := range functions {
		labels = append(labels, e.label)
	}
	if len(functions) != 2 || functions[0].line != 10 || functions[0].label != "alpha" || functions[1].line != 2500 || functions[1].label != "beta" {
		t.Fatalf("functions %q at %+v, want alpha on 10 and beta on 2500", labels, functions)
	}
	if len(errs) != 1 || errs[0].line != 20 || errs[0].label != "???" {
		t.Fatalf("errors %+v, want ??? on line 20", errs)
	}

	ev.CursorLine, ev.CursorPos = 20, 6 // inside "beta"
	ev.ColorerLocateFunction()
	pumpUntil(t, "locate function", func() bool { return ch.outlineBuild == nil && !ch.pending && ev.CursorLine == 2500 })
	if ev.CursorPos != 3 {
		t.Errorf("cursor at %d:%d, want 2500:3", ev.CursorLine, ev.CursorPos)
	}
}
