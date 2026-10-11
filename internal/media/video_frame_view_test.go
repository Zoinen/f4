package media

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFrameVideoViewBackgroundFollowsViewerTheme(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	previous := vtui.Palette[theme.ColViewerText]
	t.Cleanup(func() { vtui.Palette[theme.ColViewerText] = previous })
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(8, 4)
	// An already-started run with no frame isolates the background paint.
	fv := &FrameVideoView{cancel: func() {}, runCols: 8, runRows: 3}
	fv.SetPosition(0, 0, 7, 3)
	for _, background := range []uint32{0x123456, 0xabcdef} {
		attr := vtui.SetRGBBoth(0, 0xffffff, background)
		vtui.Palette[theme.ColViewerText] = attr
		fv.Show(scr)
		for y := 1; y <= 3; y++ {
			for x := 0; x < 8; x++ {
				cell := scr.GetCell(x, y)
				if cell.Char != ' ' || cell.Attributes != attr {
					t.Fatalf("background at (%d,%d) = %+v, want blank with %#x", x, y, cell, attr)
				}
			}
		}
	}
}

func TestFrameViewSpecSizesThePictureToTheScreen(t *testing.T) {
	text := vtui.NewScreenBuf()
	text.Writer = io.Discard
	text.AllocBuf(80, 25)
	spec, graphics := frameViewSpec(text, 40, 10, 3*time.Second)
	if graphics || spec.Width != 40 || spec.Height != 20 || spec.FPS != frameViewTextFPS || spec.Start != 3*time.Second {
		t.Fatalf("text spec = %+v graphics=%v, want 40x20 at %d fps", spec, graphics, frameViewTextFPS)
	}

	gfx := newImageTestScreen(t) // kitty, 8x16 cells
	spec, graphics = frameViewSpec(gfx, 100, 40, 0)
	if !graphics || spec.Width > frameViewMaxWidth || spec.Height > frameViewMaxHeight || spec.FPS != frameViewGraphicsFPS {
		t.Fatalf("graphics spec = %+v graphics=%v", spec, graphics)
	}
	if spec.Width%2 != 0 || spec.Height%2 != 0 {
		t.Errorf("odd frame size %dx%d", spec.Width, spec.Height)
	}
	small, _ := frameViewSpec(gfx, 10, 5, 0)
	if small.Width != 80 || small.Height != 80 {
		t.Errorf("a small area asked for %dx%d, want 80x80 (its own pixels)", small.Width, small.Height)
	}
}

func TestFrameVideoViewPlaysFramesAsHalfBlocks(t *testing.T) {
	useFakeFFmpeg(t, "head -c 4800 /dev/zero") // three 20x20 frames
	scr := vtui.NewScreenBuf()
	scr.Writer = io.Discard
	scr.AllocBuf(20, 12)
	vtui.SetDefaultPalette()

	fv := NewFrameVideoView(nil, videoFileForTest(t))
	fv.topBar.ColorIdx = 0 // the theme's palette is not loaded in a unit test
	fv.ResizeConsole(20, 12)
	fv.Show(scr) // starts the run

	deadline := time.Now().Add(10 * time.Second)
	for {
		fv.mu.Lock()
		done := fv.ended && fv.frame != nil
		fv.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the frames never arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fv.Show(scr)
	var dump bytes.Buffer
	scr.Dump(&dump)
	if !strings.Contains(dump.String(), "▀") {
		t.Fatalf("no half blocks on the screen:\n%s", dump.String())
	}
	if !strings.Contains(fv.statusText(), "end") {
		t.Errorf("status %q does not say the film ended", fv.statusText())
	}

	// A resized screen restarts the run from where it was.
	fv.ResizeConsole(30, 12)
	fv.Show(scr)
	fv.mu.Lock()
	cols := fv.runCols
	fv.mu.Unlock()
	if cols != 30 {
		t.Errorf("the run was not restarted for the new size (cols %d)", cols)
	}

	if !fv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE}) || !fv.isPaused() {
		t.Error("space did not pause")
	}
	closed := 0
	fv.OnClose = func() { closed++ }
	if !fv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}) || !fv.IsDone() || closed != 1 {
		t.Errorf("escape did not close the view (done=%v, callbacks=%d)", fv.IsDone(), closed)
	}
	// The stand-in for ffmpeg is put back when the test ends; no run may still
	// be looking for it then.
	fv.runs.Wait()
}

func TestFrameVideoViewReportsAFailedStartAndIgnoresOtherKeys(t *testing.T) {
	useFakeFFmpeg(t, "exit 3")
	scr := vtui.NewScreenBuf()
	scr.Writer = io.Discard
	scr.AllocBuf(20, 12)
	vtui.SetDefaultPalette()
	fv := NewFrameVideoView(nil, videoFileForTest(t))
	fv.topBar.ColorIdx = 0
	fv.ResizeConsole(20, 12)
	fv.Show(scr)
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(fv.statusText(), "exit") {
		if time.Now().After(deadline) {
			t.Fatalf("status %q never reported the failure", fv.statusText())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if fv.ProcessKey(nil) || fv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_B}) {
		t.Error("an unrelated key was handled")
	}
	fv.Close()
	fv.runs.Wait()
}

// videoFileForTest is a file that exists, which is all the frame source asks of
// a video before it hands it to ffmpeg.
func videoFileForTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("not really a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVideoSiblingsWalkThroughTheFilms(t *testing.T) {
	var got []string
	s := &VideoSiblings{Paths: []string{"a.mp4", "b.mp4", "c.mp4"}, Index: 1, OnStep: func(p string) { got = append(got, p) }}
	for _, vk := range []uint16{vtinput.VK_NEXT, vtinput.VK_PRIOR, vtinput.VK_HOME, vtinput.VK_END} {
		if !s.step(vk) {
			t.Fatalf("key %d not handled", vk)
		}
	}
	if want := []string{"c.mp4", "a.mp4", "a.mp4", "c.mp4"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("stepped to %v, want %v", got, want)
	}
	// Wrapping, and standing still on the film already shown.
	got = nil
	s.Index = 2
	s.step(vtinput.VK_NEXT)
	s.Index = 0
	s.step(vtinput.VK_PRIOR)
	s.step(vtinput.VK_HOME) // already the first: handled, nowhere to go
	if strings.Join(got, ",") != "a.mp4,c.mp4" {
		t.Fatalf("wrapping steps = %v", got)
	}
	if s.step(vtinput.VK_A) {
		t.Error("an unrelated key stepped")
	}
	for _, alone := range []*VideoSiblings{nil, {Paths: []string{"a"}, Index: 0, OnStep: func(string) {}}, {Paths: []string{"a", "b"}, Index: -1, OnStep: func(string) {}}, {Paths: []string{"a", "b"}, Index: 0}} {
		if alone.step(vtinput.VK_NEXT) {
			t.Errorf("%+v stepped with nowhere to go", alone)
		}
	}

	// And through the frame's own key handling.
	fv := NewFrameVideoView(nil, "a.mp4")
	fv.Siblings = &VideoSiblings{Paths: []string{"a.mp4", "b.mp4"}, Index: 0, OnStep: func(p string) { got = append(got[:0], p) }}
	if !fv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT}) || len(got) != 1 || got[0] != "b.mp4" {
		t.Fatalf("PgDn on the frame: %v", got)
	}
}
