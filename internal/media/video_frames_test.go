package media

import (
	"bytes"
	"context"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVideoFrameArgsShape(t *testing.T) {
	args := videoFrameArgs("/films/a.mkv", VideoFrameSpec{Width: 80, Height: 48, FPS: 10, Start: 90*time.Second + 500*time.Millisecond})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-ss 90.500 -i /films/a.mkv",
		"-vf fps=10,scale=80:48:force_original_aspect_ratio=decrease,pad=80:48:(ow-iw)/2:(oh-ih)/2:black",
		"-pix_fmt rgba", "-f rawvideo -", "-an",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("ffmpeg arguments %q lack %q", joined, want)
		}
	}
	if strings.Contains(strings.Join(videoFrameArgs("a", VideoFrameSpec{Width: 2, Height: 2, FPS: 1}), " "), "-ss") {
		t.Error("a source that starts at zero was given a seek")
	}
}

func TestReadVideoFramesTimesFramesAndDropsATruncatedOne(t *testing.T) {
	s := VideoFrameSpec{Width: 2, Height: 2, FPS: 4, Start: time.Second}
	stream := bytes.Repeat([]byte{7}, s.FrameBytes()*3+5) // three frames and a stub
	var got []VideoFrame
	if err := readVideoFrames(bytes.NewReader(stream), s, func(f VideoFrame) bool {
		got = append(got, f)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%d frames, want 3", len(got))
	}
	for i, f := range got {
		if want := time.Second + time.Duration(i)*250*time.Millisecond; f.PTS != want {
			t.Errorf("frame %d at %v, want %v", i, f.PTS, want)
		}
		if f.Width != 2 || f.Height != 2 || len(f.Pix) != 16 {
			t.Errorf("frame %d is %dx%d with %d bytes", i, f.Width, f.Height, len(f.Pix))
		}
	}

	count := 0
	if err := readVideoFrames(bytes.NewReader(stream), s, func(VideoFrame) bool { count++; return false }); err != nil || count != 1 {
		t.Errorf("stopping at the first frame: %d frames, err %v", count, err)
	}
	if err := readVideoFrames(bytes.NewReader(nil), VideoFrameSpec{}, nil); err == nil {
		t.Error("a zero-size spec was accepted")
	}
	if _, err := OpenVideoFrames(context.Background(), "x", VideoFrameSpec{Width: 2, Height: 2}); err == nil {
		t.Error("a spec with no frame rate was accepted")
	}
}

// fakeFFmpeg writes a shell script standing in for ffmpeg.
func fakeFFmpeg(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in for ffmpeg is a shell script")
	}
	bin := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil { //nolint:gosec // a test stand-in the test itself runs
		t.Fatal(err)
	}
	return bin
}

func TestVideoFrameSourceReadsFramesFromTheProcess(t *testing.T) {
	s := VideoFrameSpec{Width: 2, Height: 2, FPS: 5}
	src, err := startVideoFrames(context.Background(), fakeFFmpeg(t, "head -c 40 /dev/zero"), "film.mp4", s)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	n := 0
	for range src.Frames {
		n++
	}
	if n != 2 || src.Err() != nil {
		t.Fatalf("%d frames, err %v; want 2 and none", n, src.Err())
	}
}

func TestVideoFrameSourceReportsAFailingFFmpegAndCloses(t *testing.T) {
	s := VideoFrameSpec{Width: 2, Height: 2, FPS: 5}
	src, err := startVideoFrames(context.Background(), fakeFFmpeg(t, "exit 3"), "film.mp4", s)
	if err != nil {
		t.Fatal(err)
	}
	for range src.Frames {
	}
	if src.Err() == nil {
		t.Error("an ffmpeg that failed left no error")
	}
	src.Close()

	endless, err := startVideoFrames(context.Background(), fakeFFmpeg(t, "while :; do head -c 16 /dev/zero; done"), "film.mp4", s)
	if err != nil {
		t.Fatal(err)
	}
	<-endless.Frames
	done := make(chan struct{})
	go func() { endless.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not stop an endless source")
	}
	if endless.Err() != nil {
		t.Errorf("a closed source reported %v", endless.Err())
	}

	if _, err := startVideoFrames(context.Background(), filepath.Join(t.TempDir(), "nothing"), "a", s); err == nil {
		t.Error("a missing ffmpeg started")
	}
}

// useFakeFFmpeg makes the frame source find the stand-in instead of the real
// ffmpeg for the rest of the test.
func useFakeFFmpeg(t *testing.T, body string) {
	t.Helper()
	bin := fakeFFmpeg(t, body)
	key := strings.Join(ToolFFmpeg.Names, ",")
	toolPathMu.Lock()
	old, had := toolPaths[key]
	toolPaths[key] = bin
	toolPathMu.Unlock()
	t.Cleanup(func() {
		toolPathMu.Lock()
		defer toolPathMu.Unlock()
		if had {
			toolPaths[key] = old
		} else {
			delete(toolPaths, key)
		}
	})
}

func TestSaveVideoPosterWritesAPNGAndRetriesFromTheStart(t *testing.T) {
	video := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(video, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A video shorter than a second: ffmpeg has nothing after -ss 1.
	useFakeFFmpeg(t, `case "$*" in *-ss*) exit 0;; esac
head -c 921600 /dev/zero`)
	dest := filepath.Join(t.TempDir(), "poster.png")
	if err := SaveVideoPoster(context.Background(), video, dest); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil || img.Bounds().Dx() != posterWidth || img.Bounds().Dy() != posterHeight {
		t.Fatalf("poster: %v, %v", img, err)
	}
}

func TestSaveVideoPosterReportsAVideoWithNoPicture(t *testing.T) {
	video := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(video, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	useFakeFFmpeg(t, "exit 0")
	if err := SaveVideoPoster(context.Background(), video, filepath.Join(t.TempDir(), "p.png")); err == nil {
		t.Fatal("a video with no frames gave a poster")
	}
	if err := SaveVideoPoster(context.Background(), filepath.Join(t.TempDir(), "missing.mp4"), "x.png"); err == nil {
		t.Fatal("a missing video gave a poster")
	}
}
