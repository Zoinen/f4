package vtui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// restoreCursorStyle puts back the caret style a test changed; the setting
// is package-wide.
func restoreCursorStyle(t *testing.T) {
	t.Helper()
	insert, overtype, blink := InsertCursorShape(), OvertypeCursorShape(), CursorBlinks()
	t.Cleanup(func() { SetCursorStyle(insert, overtype, blink) })
}

func TestCursorStyleDefaultsKeepHistoricShapes(t *testing.T) {
	if InsertCursorShape() != CursorShapeUnderline || OvertypeCursorShape() != CursorShapeBlock || !CursorBlinks() {
		t.Fatalf("defaults = %v/%v/%v, want underline/block/blinking",
			InsertCursorShape(), OvertypeCursorShape(), CursorBlinks())
	}
}

func TestSetCursorStyleFallsBackForUnknownShapes(t *testing.T) {
	restoreCursorStyle(t)
	SetCursorStyle(CursorShape(42), CursorShape(-1), false)
	if InsertCursorShape() != CursorShapeUnderline || OvertypeCursorShape() != CursorShapeBlock {
		t.Fatalf("unknown shapes stored as %v/%v", InsertCursorShape(), OvertypeCursorShape())
	}
	if CursorBlinks() {
		t.Fatal("blink setting ignored")
	}
}

func TestParseCursorShapeRoundTrips(t *testing.T) {
	for _, shape := range []CursorShape{CursorShapeUnderline, CursorShapeBar, CursorShapeBlock} {
		got, ok := ParseCursorShape(shape.String())
		if !ok || got != shape {
			t.Errorf("ParseCursorShape(%q) = %v, %v", shape.String(), got, ok)
		}
	}
	if _, ok := ParseCursorShape("hollow"); ok {
		t.Error("unknown name accepted")
	}
}

// DECSCUSR per xterm's ctlseqs: odd Ps blink, even Ps are steady.
func TestCursorStyleSeqIsDECSCUSR(t *testing.T) {
	cases := []struct {
		shape CursorShape
		blink bool
		want  string
	}{
		{CursorShapeBlock, true, "\x1b[1 q"},
		{CursorShapeBlock, false, "\x1b[2 q"},
		{CursorShapeUnderline, true, "\x1b[3 q"},
		{CursorShapeUnderline, false, "\x1b[4 q"},
		{CursorShapeBar, true, "\x1b[5 q"},
		{CursorShapeBar, false, "\x1b[6 q"},
	}
	for _, c := range cases {
		if got := cursorStyleSeq(c.shape, c.blink); got != c.want {
			t.Errorf("cursorStyleSeq(%v, %v) = %q, want %q", c.shape, c.blink, got, c.want)
		}
	}
	for shape, want := range map[CursorShape]string{CursorShapeBlock: "0", CursorShapeBar: "1", CursorShapeUnderline: "2"} {
		if got := cursorShapeOSC1337(shape); got != "\x1b]1337;CursorShape="+want+"\x07" {
			t.Errorf("cursorShapeOSC1337(%v) = %q", shape, got)
		}
	}
}

func flushCaret(t *testing.T, shape CursorShape) (*ScreenBuf, *bytes.Buffer) {
	t.Helper()
	scr := NewScreenBuf()
	var buf bytes.Buffer
	scr.Writer = &buf
	scr.AllocBuf(10, 10)
	scr.SetCursorPos(1, 1)
	scr.SetCursorVisible(true)
	scr.SetCursorShape(shape)
	scr.Flush()
	return scr, &buf
}

func TestAnsiRendererSendsSteadyBar(t *testing.T) {
	oldTerm := os.Getenv("TERM")
	defer os.Setenv("TERM", oldTerm)
	os.Setenv("TERM", "xterm-256color")
	restoreCursorStyle(t)
	SetCursorStyle(CursorShapeBar, CursorShapeUnderline, false)

	_, buf := flushCaret(t, CursorShapeBar)
	out := buf.String()
	for _, want := range []string{"\x1b[6 q", "\x1b]1337;CursorShape=1\x07"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q: %q", want, out)
		}
	}
}

// Turning blinking off changes neither the caret's position nor its shape,
// and must still reach the terminal.
func TestAnsiRendererResendsWhenOnlyBlinkChanges(t *testing.T) {
	oldTerm := os.Getenv("TERM")
	defer os.Setenv("TERM", oldTerm)
	os.Setenv("TERM", "xterm-256color")
	restoreCursorStyle(t)
	SetCursorStyle(CursorShapeUnderline, CursorShapeBlock, true)

	scr, buf := flushCaret(t, CursorShapeUnderline)
	if !strings.Contains(buf.String(), "\x1b[3 q") {
		t.Fatalf("first frame lacks blinking underline: %q", buf.String())
	}
	buf.Reset()
	scr.Flush()
	if strings.Contains(buf.String(), " q") {
		t.Fatalf("unchanged caret resent: %q", buf.String())
	}

	SetCursorStyle(CursorShapeUnderline, CursorShapeBlock, false)
	scr.Flush()
	if !strings.Contains(buf.String(), "\x1b[4 q") {
		t.Fatalf("steady underline not sent after the blink change: %q", buf.String())
	}
}

func TestEditAsksForConfiguredCaret(t *testing.T) {
	restoreCursorStyle(t)
	SetCursorStyle(CursorShapeBar, CursorShapeUnderline, true)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 3)
	e := NewEdit(0, 0, 10, "abc")
	e.SetFocus(true)

	e.Show(scr)
	if _, _, _, shape := scr.GetCursorStateForTesting(); shape != CursorShapeBar {
		t.Fatalf("insert caret = %v, want bar", shape)
	}
	e.overtype = true
	e.Show(scr)
	if _, _, _, shape := scr.GetCursorStateForTesting(); shape != CursorShapeUnderline {
		t.Fatalf("overtype caret = %v, want underline", shape)
	}
}

func TestCursorCellRect(t *testing.T) {
	cases := []struct {
		shape          CursorShape
		scaled         bool
		x0, y0, x1, y1 int
	}{
		{CursorShapeBlock, false, 0, 0, 8, 16},
		{CursorShapeUnderline, false, 0, 14, 8, 16},
		{CursorShapeUnderline, true, 0, 12, 8, 16},
		{CursorShapeBar, false, 0, 0, 2, 16},
		{CursorShapeBar, true, 0, 0, 4, 16},
	}
	for _, c := range cases {
		x0, y0, x1, y1 := cursorCellRect(c.shape, 8, 16, c.scaled)
		if x0 != c.x0 || y0 != c.y0 || x1 != c.x1 || y1 != c.y1 {
			t.Errorf("cursorCellRect(%v, scaled=%v) = %d,%d-%d,%d, want %d,%d-%d,%d",
				c.shape, c.scaled, x0, y0, x1, y1, c.x0, c.y0, c.x1, c.y1)
		}
	}
	// A cell narrower than the bar is clipped to the cell.
	if _, _, x1, _ := cursorCellRect(CursorShapeBar, 1, 16, true); x1 != 1 {
		t.Errorf("bar not clipped to a one-pixel cell: x1=%d", x1)
	}
}

func TestInvertCursorRectPaintsOnlyTheBar(t *testing.T) {
	const w, h = 4, 3
	pix := make([]uint8, w*h*4)
	invertCursorRect(pix, w*4, w, h, 0, 0, CursorShapeBar, w, h, false)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			inverted := pix[(y*w+x)*4] == 255
			if inverted != (x < 2) {
				t.Errorf("pixel %d,%d inverted=%v", x, y, inverted)
			}
		}
	}
}

func TestStepSoftwareBlink(t *testing.T) {
	restoreCursorStyle(t)
	start := time.Unix(1000, 0)

	SetCursorStyle(CursorShapeUnderline, CursorShapeBlock, true)
	visible, last := true, start
	stepSoftwareBlink(&visible, &last, start.Add(softwareBlinkPeriod))
	if visible {
		t.Fatal("blinking caret did not go dark after a half-period")
	}

	SetCursorStyle(CursorShapeUnderline, CursorShapeBlock, false)
	stepSoftwareBlink(&visible, &last, start.Add(2*softwareBlinkPeriod))
	if !visible {
		t.Fatal("steady caret left in its dark phase")
	}
	stepSoftwareBlink(&visible, &last, start.Add(10*softwareBlinkPeriod))
	if !visible {
		t.Fatal("steady caret blinked")
	}
}

func TestResumeSendsConfiguredInsertCaret(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	oldVia := cursorStyleViaConsoleAPI
	cursorStyleViaConsoleAPI = func() bool { return false }
	defer func() { cursorStyleViaConsoleAPI = oldVia }()

	oldEnable := enableTerminalInput
	enableTerminalInput = func() (func(), error) { return func() {}, nil }
	defer func() { enableTerminalInput = oldEnable }()

	oldUsesVT := consoleUsesVT
	consoleUsesVT = func() bool { return true }
	defer func() { consoleUsesVT = oldUsesVT }()

	oldFreeBSD := IsFreeBSDConsole
	IsFreeBSDConsole = false
	defer func() { IsFreeBSDConsole = oldFreeBSD }()

	restoreCursorStyle(t)
	SetCursorStyle(CursorShapeBar, CursorShapeBlock, true)
	ManageCursorStyle = true
	isPrepared = false
	inAltScreen = false
	inputRestore = func() {}
	defer func() { isPrepared = false; inAltScreen = false; inputRestore = nil }()

	_ = Resume()
	if out := mock.builder.String(); !strings.Contains(out, "\x1b[5 q") {
		t.Fatalf("Resume did not send the blinking bar: %q", out)
	}
}
