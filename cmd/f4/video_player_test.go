package main

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/ttyx"
)

func TestIsVideoFile(t *testing.T) {
	for _, name := range []string{"a.mp4", "B.MKV", "clip.webm", "x.avi", "y.MOV"} {
		if !IsVideoFile(name) {
			t.Errorf("%s is a video", name)
		}
	}
	for _, name := range []string{"a.png", "b.txt", "c", "d.mp4x", "e.jpg"} {
		if IsVideoFile(name) {
			t.Errorf("%s is not a video", name)
		}
	}
}

// The player draws into f4's own window and is driven down a socket, because
// the window has no keyboard: it is override-redirect and shaped out of the
// input region so that the terminal underneath keeps working.
func TestVideoArgs(t *testing.T) {
	args := videoArgs("/tmp/a b.mp4", "/tmp/sock", 12345)
	joined := strings.Join(args, " ")

	for _, want := range []string{"--wid=12345", "--input-ipc-server=/tmp/sock", "--no-border"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %v", want, args)
		}
	}
	// The path goes last and after a bare --, so a file called --version is
	// a file and not an option.
	if args[len(args)-1] != "/tmp/a b.mp4" || args[len(args)-2] != "--" {
		t.Errorf("the file must come last, behind a --: %v", args[len(args)-3:])
	}
}

// Every method has to survive a nil player: that is what "mpv is not here"
// looks like to the frame.
func TestVideoPlayerNilIsSafe(t *testing.T) {
	var p *videoPlayer
	p.Close()
	p.TogglePause()
	p.Seek(10)
	p.Volume(5)
	p.SetPaused(true)
	p.Place(ttyx.Rect{})
	if p.Done() != nil {
		t.Error("a player that does not exist is never done")
	}
}
