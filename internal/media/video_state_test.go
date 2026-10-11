package media

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestPlaybackTrackerReadsPropertyChanges(t *testing.T) {
	var tr playbackTracker
	if tr.snapshot().Known {
		t.Fatal("a tracker that heard nothing claims to know")
	}
	for _, line := range []string{
		`{"event":"property-change","id":1,"name":"time-pos","data":65.4}`,
		`{"event":"property-change","id":2,"name":"duration","data":2530}`,
		`{"event":"property-change","id":3,"name":"pause","data":true}`,
		`{"event":"property-change","id":4,"name":"volume","data":79.6}`,
		`{"error":"success","data":null}`, // a reply, not an event
		`not json`,
		`{"event":"property-change","name":"time-pos","data":null}`,
	} {
		tr.apply([]byte(line))
	}
	got := tr.snapshot()
	if !got.Known || got.Duration != 2530*time.Second || !got.Paused || got.Volume != 80 || got.Position != 0 {
		t.Fatalf("state = %+v (the last line said the position is null: zero)", got)
	}
	tr.apply([]byte(`{"event":"property-change","name":"time-pos","data":65.4}`))
	if got := tr.snapshot(); got.statusText() != " 1:05 / 42:10 │ paused │ vol 80 " {
		t.Errorf("status = %q", got.statusText())
	}
	if (PlaybackState{}).statusText() != "" {
		t.Error("an unknown state has a status text")
	}
	if formatClock(3725*time.Second) != "1:02:05" || formatClock(-time.Second) != "0:00" {
		t.Errorf("clock: %q %q", formatClock(3725*time.Second), formatClock(-time.Second))
	}
	if (*videoPlayer)(nil).State().Known {
		t.Error("a nil player knows something")
	}
}

// shortSocket is a socket path short enough for a unix socket.
func shortSocket(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("mpv's socket is a unix socket here")
	}
	dir, err := os.MkdirTemp("", "f4v")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

// fakeMPV listens on a socket and reports every line a client sends.
func fakeMPV(t *testing.T, sock string) (lines chan string, serve func(func(net.Conn))) {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	lines = make(chan string, 32)
	return lines, func(handle func(net.Conn)) {
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					defer func() { _ = conn.Close() }()
					if handle != nil {
						handle(conn)
					}
					sc := bufio.NewScanner(conn)
					for sc.Scan() {
						lines <- sc.Text()
					}
				}()
			}
		}()
	}
}

func TestVideoPlayerObservesPropertiesOverTheSocket(t *testing.T) {
	sock := shortSocket(t)
	lines, serve := fakeMPV(t, sock)
	serve(func(conn net.Conn) {
		_, _ = conn.Write([]byte(`{"event":"property-change","name":"time-pos","data":5}` + "\n"))
	})
	p := &videoPlayer{sock: sock, done: make(chan struct{})}
	p.startObserving()

	for _, name := range observedProperties {
		select {
		case l := <-lines:
			if !strings.Contains(l, `"observe_property"`) || !strings.Contains(l, `"`+name+`"`) {
				t.Fatalf("asked %q, want an observe_property for %s", l, name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no observe_property for %s", name)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for p.State().Position != 5*time.Second {
		if time.Now().After(deadline) {
			t.Fatalf("the position never arrived: %+v", p.State())
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(p.done)
}

func TestVideoViewKeysBecomeReviewCommands(t *testing.T) {
	sock := shortSocket(t)
	lines, serve := fakeMPV(t, sock)
	serve(nil)
	vv, _ := NewVideoView(nil, "clip.mp4")
	vv.player = &videoPlayer{sock: sock}

	press := func(vk uint16, mods vtinput.ControlKeyState) string {
		t.Helper()
		if !vv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vk, ControlKeyState: mods}) {
			t.Fatalf("key %d/%d not handled", vk, mods)
		}
		select {
		case l := <-lines:
			return l
		case <-time.After(5 * time.Second):
			t.Fatalf("no command for key %d/%d", vk, mods)
		}
		return ""
	}
	const ctrl, shift = vtinput.LeftCtrlPressed, vtinput.ShiftPressed
	for _, tc := range []struct {
		vk   uint16
		mods vtinput.ControlKeyState
		want string
	}{
		{vtinput.VK_RIGHT, 0, `{"command":["seek",10,"relative"]}`},
		{vtinput.VK_LEFT, shift, `{"command":["seek",-1,"relative"]}`},
		{vtinput.VK_RIGHT, ctrl, `{"command":["seek",100,"absolute-percent"]}`},
		{vtinput.VK_LEFT, ctrl, `{"command":["seek",0,"absolute"]}`},
		{vtinput.VK_UP, 0, `{"command":["add","volume",10]}`},
		{vtinput.VK_DOWN, shift, `{"command":["add","volume",-2]}`},
		{vtinput.VK_UP, ctrl, `{"command":["set_property","volume",100]}`},
		{vtinput.VK_DOWN, ctrl, `{"command":["set_property","volume",0]}`},
		{vtinput.VK_A, 0, `{"command":["cycle","audio"]}`},
		{vtinput.VK_A, ctrl, `{"command":["cycle","audio"]}`},
		{vtinput.VK_A, ctrl | shift, `{"command":["cycle","audio","down"]}`},
	} {
		if got := press(tc.vk, tc.mods); got != tc.want {
			t.Errorf("key %d/%d sent %s, want %s", tc.vk, tc.mods, got, tc.want)
		}
	}
	if vv.topBar == nil {
		t.Fatal("no title bar")
	}
}
