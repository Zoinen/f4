package panel

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPlayerFileHelpers(t *testing.T) {
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "recording.mp3")
	wav := filepath.Join(dir, "take.WAV")
	text := filepath.Join(dir, "notes.txt")
	for _, path := range []string{mp3, wav, text} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := trackDisplayName(mp3); got != "recording" {
		t.Fatalf("trackDisplayName(mp3) = %q, want recording", got)
	}
	if got := trackDisplayName(wav); got != "take" {
		t.Fatalf("trackDisplayName(wav) = %q, want take", got)
	}
	if item := playlistItemForFile(text); item != nil {
		t.Fatalf("playlistItemForFile(text) = %#v, want nil", item)
	}
	item := playlistItemForFile(mp3)
	if item == nil || item.Path != mp3 || item.Name != "recording" {
		t.Fatalf("playlistItemForFile(mp3) = %#v", item)
	}

	if got := fmtClock(125*time.Second + 900*time.Millisecond); got != "02:05" {
		t.Fatalf("fmtClock = %q, want 02:05", got)
	}
	for level, want := range map[int]string{-1: " ", 0: " ", 8: "█", 20: "█"} {
		if got := eighthBlock(level); got != want {
			t.Errorf("eighthBlock(%d) = %q, want %q", level, got, want)
		}
	}
}

func TestFillFolderFromDirSkipsEmptyAndUnsupported(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "song.mp3"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	folder := &playlistItem{Name: "root", Folder: true}
	if got := fillFolderFromDir(folder, dir); got != 1 {
		t.Fatalf("fillFolderFromDir = %d, want 1", got)
	}
	if len(folder.Children) != 1 || folder.Children[0].Name != "nested" {
		t.Fatalf("children = %v, want nested folder only", names(folder.Children))
	}
	if got := fillFolderFromDir(folder, filepath.Join(dir, "missing")); got != 0 {
		t.Fatalf("missing directory count = %d, want 0", got)
	}
}

func TestStopIfPlayingFiltersQueue(t *testing.T) {
	pp := newTestPlayerPanel()
	pp.current = &playlistItem{Path: "/music/keep.mp3"}
	pp.queue = []string{"/music/drop-a.mp3", "/music/keep.mp3", "/music/drop-b.mp3"}
	pp.queuePos = 1

	pp.StopIfPlaying([]string{"/music/drop-a.mp3", "/music/drop-b.mp3"})
	if len(pp.queue) != 1 || pp.queue[0] != "/music/keep.mp3" || pp.queuePos != 0 {
		t.Fatalf("queue after filtering = %v pos=%d", pp.queue, pp.queuePos)
	}
	if pp.current == nil || pp.current.Path != "/music/keep.mp3" {
		t.Fatalf("current track changed unexpectedly: %#v", pp.current)
	}

	pp.StopIfPlaying([]string{"/music/keep.mp3"})
	if pp.current != nil || len(pp.queue) != 0 || pp.queuePos != -1 {
		t.Fatalf("queue after stopping current = %v pos=%d current=%#v", pp.queue, pp.queuePos, pp.current)
	}
}
