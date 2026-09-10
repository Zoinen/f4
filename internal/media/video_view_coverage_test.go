package media

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"path/filepath"
	"testing"
)

func newCoverageVideoView(t *testing.T) *VideoView {
	t.Helper()
	vv, err := NewVideoView(nil, "/tmp/video clip.mp4")
	if err != nil {
		t.Fatalf("NewVideoView: %v", err)
	}
	// A player without a process is enough to exercise the frame's input
	// routing. Its socket is deliberately absent, so commands fail harmlessly
	// without starting mpv or needing an X session.
	vv.player = &videoPlayer{sock: filepath.Join(t.TempDir(), "missing.sock")}
	return vv
}

func videoViewKey(vk uint16, down bool) *vtinput.InputEvent {
	return &vtinput.InputEvent{KeyDown: down, VirtualKeyCode: vk}
}

func TestVideoViewConstructionAndLayout(t *testing.T) {
	vv, err := NewVideoView(nil, "/tmp/video clip.mp4")
	if err != nil {
		t.Fatalf("NewVideoView: %v", err)
	}
	if got := vv.topBar.GetLeft(); got != " video clip.mp4" {
		t.Fatalf("title callback returned %q", got)
	}

	vv.SetPosition(2, 3, 20, 10)
	if x1, y1, x2, y2 := vv.GetPosition(); x1 != 2 || y1 != 3 || x2 != 20 || y2 != 10 {
		t.Fatalf("frame position = %d,%d,%d,%d", x1, y1, x2, y2)
	}
	if x1, y1, x2, y2 := vv.topBar.GetPosition(); x1 != 2 || y1 != 3 || x2 != 20 || y2 != 3 {
		t.Fatalf("top bar position = %d,%d,%d,%d", x1, y1, x2, y2)
	}

	vv.ResizeConsole(80, 25)
	if x1, y1, x2, y2 := vv.GetPosition(); x1 != 0 || y1 != 0 || x2 != 79 || y2 != 23 {
		t.Fatalf("resized frame position = %d,%d,%d,%d", x1, y1, x2, y2)
	}
	if labels := vv.GetKeyLabels().Normal; labels[9] != "Quit" {
		t.Fatalf("F10 label = %q", labels[9])
	}
	if got, want := vv.GetType(), vtui.TypeUser+8; got != want {
		t.Fatalf("frame type = %d, want %d", got, want)
	}
}

func TestVideoViewStartNeedsLaidOutScreenAndSession(t *testing.T) {
	vv, err := NewVideoView(nil, "clip.mp4")
	if err != nil {
		t.Fatalf("NewVideoView: %v", err)
	}
	if vv.start(nil) {
		t.Fatal("start(nil) must wait for a usable picture rectangle")
	}
	if vv.player != nil {
		t.Fatal("failed start installed a player")
	}

	vv.player = &videoPlayer{}
	if !vv.start(nil) {
		t.Fatal("start must be idempotent once a player exists")
	}
	if _, ok := vv.pictureRect(nil); ok {
		t.Fatal("pictureRect(nil) must fail")
	}
}

func TestVideoViewProcessKeyRoutesPlaybackAndClose(t *testing.T) {
	if vv := newCoverageVideoView(t); vv.ProcessKey(nil) {
		t.Fatal("nil event was handled")
	}
	if vv := newCoverageVideoView(t); vv.ProcessKey(videoViewKey(vtinput.VK_SPACE, false)) {
		t.Fatal("key-up event was handled")
	}
	vv, _ := NewVideoView(nil, "clip.mp4")
	if vv.ProcessKey(videoViewKey(vtinput.VK_SPACE, true)) {
		t.Fatal("key with no player was handled")
	}

	vv = newCoverageVideoView(t)
	if !vv.ProcessKey(videoViewKey(vtinput.VK_SPACE, true)) || !vv.paused {
		t.Fatal("space did not pause playback")
	}
	if !vv.ProcessKey(videoViewKey(vtinput.VK_SPACE, true)) || vv.paused {
		t.Fatal("second space did not resume playback")
	}
	for _, vk := range []uint16{vtinput.VK_RIGHT, vtinput.VK_LEFT, vtinput.VK_UP, vtinput.VK_DOWN} {
		if !vv.ProcessKey(videoViewKey(vk, true)) {
			t.Fatalf("playback key %d was not handled", vk)
		}
	}
	if vv.ProcessKey(videoViewKey(vtinput.VK_A, true)) {
		t.Fatal("unrelated key was handled")
	}

	for _, vk := range []uint16{vtinput.VK_ESCAPE, vtinput.VK_F10, vtinput.VK_F3} {
		vv = newCoverageVideoView(t)
		closed := 0
		vv.OnClose = func() { closed++ }
		if !vv.ProcessKey(videoViewKey(vk, true)) || !vv.IsDone() || vv.player != nil || closed != 1 {
			t.Fatalf("close key %d did not close the view: done=%v player=%v callbacks=%d", vk, vv.IsDone(), vv.player, closed)
		}
	}
}

func TestVideoViewCloseWithoutPlayer(t *testing.T) {
	vv, err := NewVideoView(nil, "clip.mp4")
	if err != nil {
		t.Fatalf("NewVideoView: %v", err)
	}
	called := false
	vv.OnClose = func() { called = true }
	vv.Close()
	if !vv.IsDone() || !called {
		t.Fatalf("Close did not finish the view: done=%v callback=%v", vv.IsDone(), called)
	}
}
