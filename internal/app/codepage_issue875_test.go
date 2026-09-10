package app

import (
	"context"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The text attached to issue #875. The single-byte samples there use a hyphen
// and the Unicode ones an em dash; encoding these reproduces the attached
// files byte for byte.
const (
	issue875Hyphen = "Go (часто также golang) - компилируемый многопоточный язык программирования, \r\n" +
		"разработанный внутри компании Google]. Использует объектно-ориентированный \r\n" +
		"(структурный) стиль с поддержкой функциональных элементов."
	issue875EmDash = "Go (часто также golang) — компилируемый многопоточный язык программирования, \r\n" +
		"разработанный внутри компании Google]. Использует объектно-ориентированный \r\n" +
		"(структурный) стиль с поддержкой функциональных элементов."
)

type issue875Sample struct {
	name string
	raw  []byte
	// codepage the file must be recognised as, and the text it has to show
	// once decoded.
	codepage int
	text     string
}

func issue875Samples(t *testing.T) []issue875Sample {
	t.Helper()

	encode := func(text string, cp int) []byte {
		raw, err := vfs.EncodeBytes([]byte(text), cp)
		if err != nil {
			t.Fatalf("encode sample in codepage %d: %v", cp, err)
		}
		return raw
	}
	// EncodeBytes writes the byte-order mark for UTF-16; utf16_le.txt is the
	// same file with it removed.
	utf16BOM := encode(issue875EmDash, 1200)

	return []issue875Sample{
		{"ansi_1251.txt", encode(issue875Hyphen, 1251), 1251, issue875Hyphen},
		{"oem_866.txt", encode(issue875Hyphen, 866), 866, issue875Hyphen},
		{"utf16_le_bom.txt", utf16BOM, 1200, issue875EmDash},
		{"utf16_le.txt", utf16BOM[2:], 1200, issue875EmDash},
		{"utf8_bom.txt", append([]byte("\ufeff"), issue875EmDash...), 65001, issue875EmDash},
		{"utf8.txt", []byte(issue875EmDash), 65001, issue875EmDash},
	}
}

func issue875WriteSamples(t *testing.T, dir string) []issue875Sample {
	t.Helper()
	samples := issue875Samples(t)
	for _, s := range samples {
		if err := os.WriteFile(filepath.Join(dir, s.name), s.raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return samples
}

// viewerText reads everything the viewer would render, running whatever the
// backend posted to the UI thread until the data is there.
func viewerText(t *testing.T, vv *viewer.ViewerView) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := vv.Backend.ReadAt(0, int(vv.Backend.Size()))
		if err == nil {
			return string(data)
		}
		if err != piecetable.ErrLoading {
			t.Fatalf("viewer ReadAt: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for viewer data")
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Every sample from the issue, opened in the viewer with auto-detect on. The
// CP1251 file used to be shown in the system ANSI codepage, and the UTF-16
// file without a byte-order mark as a binary.
func TestViewer_Issue875_OpensEverySample(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	oldState, oldAuto, oldDefault := fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage
	defer func() {
		fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage = oldState, oldAuto, oldDefault
	}()
	fileops.GlobalFileState = nil
	config.App.ViewerAutodetectCodePage = true
	config.App.ViewerDefaultCodePage = 65001

	dir := t.TempDir()
	v := vfs.NewOSVFS(dir)
	for _, s := range issue875WriteSamples(t, dir) {
		path := filepath.Join(dir, s.name)
		vv, err := viewer.NewViewerView(context.Background(), v, path)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}

		if vv.Codepage != s.codepage {
			t.Errorf("%s: auto-detected codepage = %d, want %d", s.name, vv.Codepage, s.codepage)
		}
		if vv.HexMode {
			t.Errorf("%s: opened as a binary", s.name)
		}
		if got := strings.TrimPrefix(viewerText(t, vv), "\ufeff"); got != s.text {
			t.Errorf("%s: viewer shows %q", s.name, got)
		}

		// Picking the file's own codepage by hand has to show the same
		// text: choosing CP1251 or CP866 used to leave the previous
		// decoding on screen.
		vv.ReloadWithCodepage(65001)
		vv.ReloadWithCodepage(s.codepage)
		if vv.Codepage != s.codepage {
			t.Errorf("%s: manual codepage = %d, want %d", s.name, vv.Codepage, s.codepage)
		}
		if got := strings.TrimPrefix(viewerText(t, vv), "\ufeff"); got != s.text {
			t.Errorf("%s: after choosing %d the viewer shows %q", s.name, s.codepage, got)
		}

		// And going back to auto-detect has to return to it, rather than
		// leaving the file displayed as a binary.
		vv.ReloadWithCodepage(1252)
		vv.ReloadWithAutoDetect()
		if vv.Codepage != s.codepage || vv.HexMode {
			t.Errorf("%s: back on auto-detect codepage = %d, hex = %v; want %d, false",
				s.name, vv.Codepage, vv.HexMode, s.codepage)
		}
		vv.Close()
	}
}

// A hex view the binary check chose is a guess, and the codepage the user
// picks next overrules it. A hex view the user asked for is not, and survives.
func TestViewer_Issue875_ManualCodepageLeavesGuessedHex(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	oldState, oldAuto, oldDefault := fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage
	defer func() {
		fileops.GlobalFileState, config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage = oldState, oldAuto, oldDefault
	}()
	fileops.GlobalFileState = nil
	config.App.ViewerAutodetectCodePage = true
	config.App.ViewerDefaultCodePage = 65001

	dir := t.TempDir()
	// UTF-16 with too little ASCII for the byte-order-mark-less check to
	// reach its bar: detection declines, the binary check sees bytes that
	// are not text under any single-byte codepage, and the file opens in hex.
	raw, err := vfs.EncodeBytes([]byte("これは日本語のテキストです。エンコーディングの検出を確認します。\r\n"), 1200)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "japanese_utf16.txt")
	if err := os.WriteFile(path, raw[2:], 0600); err != nil {
		t.Fatal(err)
	}

	vv, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS(dir), path)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()
	if !vv.HexMode || !vv.HexAuto {
		t.Fatalf("expected a guessed hex view, got hex = %v, auto = %v", vv.HexMode, vv.HexAuto)
	}

	vv.ReloadWithCodepage(1200)
	if vv.HexMode {
		t.Error("choosing UTF-16 by hand left the file in hex")
	}
	if got := viewerText(t, vv); !strings.Contains(got, "日本語") {
		t.Errorf("viewer shows %q after the codepage was chosen", got)
	}

	// Once the user takes over the view mode, a codepage switch must leave
	// it alone.
	vv.HexMode = true
	vv.HexAuto = false
	vv.ReloadWithCodepage(65001)
	if !vv.HexMode {
		t.Error("a codepage switch dropped the hex view the user asked for")
	}
}

// The same six files in the editor, through the real open path.
func TestEditor_Issue875_OpensEverySample(t *testing.T) {
	rig := newIssue875EditorRig(t)

	for _, s := range issue875WriteSamples(t, rig.dir) {
		path := filepath.Join(rig.dir, s.name)
		ev := rig.open(t, path)

		if ev.Codepage != s.codepage {
			t.Errorf("%s: auto-detected codepage = %d, want %d", s.name, ev.Codepage, s.codepage)
		}
		if ev.HexMode {
			t.Errorf("%s: opened as a binary", s.name)
		}
		text, err := ev.Pt.Bytes()
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := strings.TrimPrefix(string(text), "\ufeff"); got != s.text {
			t.Errorf("%s: editor holds %q", s.name, got)
		}

		ev.ReloadWithCodepage(65001)
		ev.ReloadWithCodepage(s.codepage)
		text, err = ev.Pt.Bytes()
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := strings.TrimPrefix(string(text), "\ufeff"); got != s.text {
			t.Errorf("%s: after choosing %d the editor holds %q", s.name, s.codepage, got)
		}
		ev.Close()
	}
}

// A UTF-8 byte-order mark is not part of the text, and skipping it must not
// shift the line index: the reported symptom was three bytes -- the marker's
// length -- missing from the start of every line but the first, which turned
// "разработанный" into "?зработанный" and "(структурный)" into "труктурный)".
func TestEditor_Issue875_UTF8BOMDoesNotShiftLines(t *testing.T) {
	rig := newIssue875EditorRig(t)

	path := filepath.Join(rig.dir, "utf8_bom.txt")
	if err := os.WriteFile(path, append([]byte("\ufeff"), issue875EmDash...), 0600); err != nil {
		t.Fatal(err)
	}

	ev := rig.open(t, path)
	defer ev.Close()

	if !ev.Utf8BOM {
		t.Fatal("the byte-order mark was not recorded")
	}
	text, err := ev.Pt.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != issue875EmDash {
		t.Fatalf("editor buffer = %q", string(text))
	}

	wantLines := strings.Split(issue875EmDash, "\r\n")
	if got := ev.Li.LineCount(); got != len(wantLines) {
		t.Fatalf("line count = %d, want %d", got, len(wantLines))
	}
	for i, want := range wantLines {
		start := ev.Li.GetLineOffset(i)
		if start+len(want) > ev.Pt.Size() {
			t.Fatalf("line %d starts at %d and runs past the %d byte buffer", i+1, start, ev.Pt.Size())
		}
		got, err := ev.Pt.GetRange(start, len(want))
		if err != nil {
			t.Fatalf("line %d: %v", i+1, err)
		}
		if string(got) != want {
			t.Errorf("line %d = %q, want %q", i+1, string(got), want)
		}
	}
}

type issue875EditorRig struct {
	panels *panel.PanelsFrame
	vfs    vfs.VFS
	dir    string
}

func newIssue875EditorRig(t *testing.T) *issue875EditorRig {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	oldState := fileops.GlobalFileState
	oldAuto, oldDefault := config.App.EditorAutodetectCodePage, config.App.EditorDefaultCodePage
	t.Cleanup(func() {
		fileops.GlobalFileState = oldState
		config.App.EditorAutodetectCodePage, config.App.EditorDefaultCodePage = oldAuto, oldDefault
	})
	fileops.GlobalFileState = nil
	config.App.EditorAutodetectCodePage = true
	config.App.EditorDefaultCodePage = 65001

	dir := t.TempDir()
	panels := panel.NewPanelsFrame()
	panels.ResizeConsole(120, 40)
	return &issue875EditorRig{panels: panels, vfs: vfs.NewOSVFS(dir), dir: dir}
}

// open runs the editor's own open path, the one an F4 on the panel takes, and
// returns the editor it created.
func (rig *issue875EditorRig) open(t *testing.T, path string) *editor.EditorView {
	t.Helper()
	f, err := rig.vfs.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ShowEditor(rig.panels, rig.vfs, path, f)

	ev, _ := FindOpenedEditor(rig.vfs, path)
	if ev == nil {
		t.Fatalf("no editor was opened for %s", path)
	}
	deadline := time.Now().Add(2 * time.Second)
	for ev.Indexing && time.Now().Before(deadline) {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(5 * time.Millisecond):
		}
	}
	return ev
}
