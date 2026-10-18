package vfs

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

type fakeElevationTimer struct{}

func (fakeElevationTimer) Stop() bool { return true }

// f4#1768 step 6a (akunitsyn: Far drops the rights, f4 kept them until it
// exits): the elevated dispatcher is closed after ElevationIdle without a
// request, and the next operation asks the user again. A request in the
// meantime keeps it. The clock and the timer are driven by hand; UAC is a
// dispatcher started in this process.
func TestElevatedDispatcherIsClosedWhenIdle(t *testing.T) {
	oldLaunch, oldConfirm, oldUAC := launchElevated, confirmElevation, connectsByUAC
	oldAfter, oldNow, oldIdle := elevationAfterFunc, elevationNow, ElevationIdle
	t.Cleanup(func() {
		launchElevated, confirmElevation, connectsByUAC = oldLaunch, oldConfirm, oldUAC
		elevationAfterFunc, elevationNow, ElevationIdle = oldAfter, oldNow, oldIdle
	})
	t.Setenv("TMPDIR", "/tmp") // the socket path must stay short enough for AF_UNIX
	connectsByUAC = func() bool { return true }
	served := make(chan error, 4)
	launches, asked := 0, 0
	launchElevated = func(exe, sock, token string) error {
		launches++
		go func() { served <- RunElevatedDispatcher(sock, token, ElevatedFileOps) }()
		return nil
	}
	confirmElevation = func() (bool, bool) {
		asked++
		return true, false
	}
	now := time.Unix(1_000_000, 0)
	elevationNow = func() time.Time { return now }
	var fire func()
	var armedFor time.Duration
	elevationAfterFunc = func(d time.Duration, f func()) elevationTimer {
		armedFor, fire = d, f
		return fakeElevationTimer{}
	}
	ElevationIdle = func() time.Duration { return 2 * time.Minute }

	c := &SudoClient{appPath: "f4.exe"}
	file := filepath.Join(t.TempDir(), "x.txt")
	if err := c.WriteAt(file, 0, []byte("a"), true, 0o600); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if asked != 1 || launches != 1 || fire == nil || armedFor != 2*time.Minute {
		t.Fatalf("first request: asked %d, launched %d, timer for %v", asked, launches, armedFor)
	}

	// A request 90 s later: the timer armed at the first one comes 30 s
	// after it, sees it and waits the 90 s that are left.
	first := fire
	now = now.Add(90 * time.Second)
	if _, err := c.Stat(file); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	now = now.Add(30 * time.Second)
	first()
	if c.conn == nil || !c.consented || armedFor != 90*time.Second {
		t.Fatalf("closed %v, consent %v, timer for %v: want it kept and the timer for the 90 s left",
			c.conn == nil, c.consented, armedFor)
	}

	// Two minutes without a request: closed, the dispatcher exits, the
	// consent is gone.
	now = now.Add(90 * time.Second)
	fire()
	if c.conn != nil || c.consented {
		t.Fatalf("after the idle time: closed %v, consent %v", c.conn == nil, c.consented)
	}
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("the dispatcher did not exit when it was closed")
	}

	// The next operation asks again and starts a new dispatcher.
	if _, err := c.Stat(file); err != nil {
		t.Fatalf("Stat after the idle close: %v", err)
	}
	if asked != 2 || launches != 2 {
		t.Fatalf("after the idle close: asked %d, launched %d; want the question and UAC again", asked, launches)
	}

	// 0 keeps the dispatcher until f4 exits: no timer is armed.
	ElevationIdle = func() time.Duration { return 0 }
	fire = nil
	if _, err := c.Stat(file); err != nil || fire != nil {
		t.Fatalf("with 0: Stat %v, a timer was armed %v", err, fire != nil)
	}
	_ = c.conn.Close()
}

// f4#1768 step 6b: with "do not ask again until f4 exits" ticked, the idle
// timer is off for the session, as before 6a.
func TestElevationRememberedKeepsTheDispatcher(t *testing.T) {
	oldLaunch, oldConfirm, oldUAC := launchElevated, confirmElevation, connectsByUAC
	oldAfter, oldIdle := elevationAfterFunc, ElevationIdle
	t.Cleanup(func() {
		launchElevated, confirmElevation, connectsByUAC = oldLaunch, oldConfirm, oldUAC
		elevationAfterFunc, ElevationIdle = oldAfter, oldIdle
	})
	t.Setenv("TMPDIR", "/tmp")
	connectsByUAC = func() bool { return true }
	served := make(chan error, 1)
	launchElevated = func(exe, sock, token string) error {
		go func() { served <- RunElevatedDispatcher(sock, token, ElevatedFileOps) }()
		return nil
	}
	confirmElevation = func() (bool, bool) { return true, true }
	armed := 0
	elevationAfterFunc = func(time.Duration, func()) elevationTimer {
		armed++
		return fakeElevationTimer{}
	}
	ElevationIdle = func() time.Duration { return 2 * time.Minute }

	c := &SudoClient{appPath: "f4.exe"}
	file := filepath.Join(t.TempDir(), "x.txt")
	if err := c.WriteAt(file, 0, []byte("a"), true, 0o600); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if _, err := c.Stat(file); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if armed != 0 || !c.remember {
		t.Fatalf("remembered consent: the idle timer was armed %d times, remember %v", armed, c.remember)
	}
	_ = c.conn.Close()
	<-served
}

// The question (f4#1768 step 6b): the text, the box off, the two buttons.
//
// Screen Dump (80x25, english; the dialog only):
//
//	╔═══════════════════════ Administrator rights ════════════════════════╗
//	║                                                                     ║
//	║ This operation needs administrator rights. If you agree, Windows    ║
//	║ asks for permission, and f4 keeps these rights while it needs them; ║
//	║ after a pause without such operations (two minutes by default) it   ║
//	║ drops them and asks again.                                          ║
//	║                                                                     ║
//	║ [ ] Do not ask again until f4 exits                                 ║
//	║                                                                     ║
//	║              [ Continue as administrator ]  [ Cancel ]              ║
//	║                                                                     ║
//	╚═════════════════════════════════════════════════════════════════════╝
func TestElevationDialogHasTheBoxOffAndTheButtons(t *testing.T) {
	oldFM := vtui.FrameManager
	t.Cleanup(func() { vtui.FrameManager = oldFM })
	vtui.FrameManager = &vtui.FrameManagerType{}
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	oldStrings := vtui.SnapshotStrings()
	t.Cleanup(func() { vtui.ReplaceStrings(oldStrings) })
	en := i18n.LoadEmbeddedLanguageMap("en")
	strs := map[string]string{}
	for _, key := range []string{"Elevation.Title", "Elevation.Text", "Elevation.Continue", "Elevation.Remember", "vtui.Cancel"} {
		strs[key] = en[key]
	}
	vtui.AddStrings(strs)

	dlg, box := newElevationDialog()
	if box.State != 0 {
		t.Errorf("the box is ticked by default")
	}
	dlg.Show(scr)
	var buf bytes.Buffer
	scr.Dump(&buf)
	dump := buf.String()
	for _, want := range []string{"[ ] " + strings.TrimPrefix(vtui.Msg("Elevation.Remember"), "&"), "Cancel", strings.TrimPrefix(vtui.Msg("Elevation.Continue"), "&")} {
		if !strings.Contains(dump, want) {
			t.Errorf("the question has no %q:\n%s", want, dump)
		}
	}
	if dlg.X2-dlg.X1+1 > 72 {
		t.Errorf("the question is %d columns wide, more than 72", dlg.X2-dlg.X1+1)
	}
	vtui.AssertLayout(t, dlg)
}
