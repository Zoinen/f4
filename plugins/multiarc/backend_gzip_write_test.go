package multiarc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGzipCheckWrite(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"gzip": true}}
	f.install(t)
	b := gzipBackend{}
	ctx := context.Background()
	if err := b.checkWrite(ctx, "/logs/app.log.gz", writeReplace, "app.log"); err != nil {
		t.Errorf("replacing the one member: %v", err)
	}
	refusals := []struct {
		op     writeOp
		member string
		want   string
	}{
		{writeAdd, "other.log", "cannot take another"},
		{writeMkDir, "dir", "cannot hold a directory"},
		{writeRemove, "app.log", "delete app.log.gz itself"},
	}
	for _, r := range refusals {
		if err := b.checkWrite(ctx, "/logs/app.log.gz", r.op, r.member); err == nil || !strings.Contains(err.Error(), r.want) {
			t.Errorf("checkWrite(%v, %q) = %v, want %q", r.op, r.member, err, r.want)
		}
	}
	if err := b.remove(ctx, "/logs/app.log.gz", []string{"app.log"}); err == nil {
		t.Error("remove should be refused")
	}
}

func TestGzipWriteNeedsGzipNotGunzip(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"gunzip": true}}
	f.install(t)
	if err := (gzipBackend{bin: "gunzip"}).checkWrite(context.Background(), "/a.log.gz", writeReplace, "a.log"); !errors.Is(err, errNoGzip) {
		t.Fatalf("checkWrite = %v, want errNoGzip", err)
	}
}

// New content for the one member is compressed next to the archive under
// the member's own name and renamed over the .gz.
func TestGzipAddRecompresses(t *testing.T) {
	arc := fakeArchive(t, "app.log.gz", "OLD")
	stage := t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "app.log"), []byte("new content"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fakeArchiver{tools: map[string]bool{"gzip": true}}
	f.install(t)

	if err := (gzipBackend{bin: "gzip"}).add(context.Background(), arc, stage, []string{"app.log"}, []string{"app.log"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if got := f.commands(); !reflect.DeepEqual(got, []string{"gzip -f -- app.log"}) {
		t.Fatalf("commands = %q", got)
	}
	// The fake gzip only renames, so the "compressed" archive holds the
	// staged bytes as they were.
	if got := readArchive(t, arc); got != "new content" {
		t.Fatalf("archive = %q", got)
	}
	assertNoScratchLeft(t, arc)
}
