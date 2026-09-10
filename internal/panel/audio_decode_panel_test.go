package panel

import (
	"testing"

	"os"
	"path/filepath"
)

func TestPlayerStopIfPlayingTrimsQueue(t *testing.T) {
	pp := newTestPlayerPanel()
	pp.queue = []string{"/r/1.amr", "/r/2.amr", "/r/3.amr", "/r/4.amr"}
	pp.queuePos = 2
	pp.current = &playlistItem{Name: "3", Path: "/r/3.amr"}

	// Deleting an earlier file shifts the position; the current one stays.
	pp.StopIfPlaying([]string{"/r/1.amr"})
	if pp.current == nil || pp.queuePos != 1 || len(pp.queue) != 3 {
		t.Fatalf("after deleting 1: pos=%d queue=%v current=%v", pp.queuePos, pp.queue, pp.current)
	}
	// Deleting the playing file stops it and drops it from the queue.
	pp.StopIfPlaying([]string{"/r/3.amr"})
	if pp.current != nil || len(pp.queue) != 2 || pp.queue[1] != "/r/4.amr" {
		t.Fatalf("after deleting 3: queue=%v current=%v", pp.queue, pp.current)
	}
	// An unrelated file changes nothing.
	pp.StopIfPlaying([]string{"/elsewhere.mp3"})
	if len(pp.queue) != 2 {
		t.Fatalf("queue=%v", pp.queue)
	}
}

func TestPlaylistItemForFileAcceptsRecordings(t *testing.T) {
	dir := t.TempDir()
	amr := filepath.Join(dir, "2026-09-06 12.00.amr")
	if err := os.WriteFile(amr, []byte("#!AMR\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	it := playlistItemForFile(amr)
	if it == nil || it.Name != "2026-09-06 12.00" {
		t.Errorf("item=%+v", it)
	}
	if playlistItemForFile(filepath.Join(dir, "notes.txt")) != nil {
		t.Errorf("text file accepted")
	}
}
