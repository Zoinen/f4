package multiarc

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestZipAddPrefersInfoZip(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"zip": true, "7z": true}}
	f.install(t)
	if err := (zipBackend{}).add(context.Background(), "/a/x.zip", "/stage", []string{"dir/[x].txt"}, []string{"dir/[x].txt"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	c := f.calls[0]
	want := []string{"-q", "-nw", "/a/x.zip", "--", "dir/[x].txt"}
	if len(f.calls) != 1 || c.name != "zip" || c.dir != "/stage" || !reflect.DeepEqual(c.args, want) {
		t.Fatalf("calls = %#v, want one zip %v in /stage", f.calls, want)
	}
}

// zip -d does not reach into a directory through its "dir/" entry, so it is
// given every raw name under it.
func TestZipRemovePassesEveryRawName(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"zip": true}}
	f.install(t)
	raws := []string{"dir/", "dir/a", "dir/sub/", "dir/sub/b"}
	if err := (zipBackend{}).remove(context.Background(), "/x.zip", raws); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := f.commands(); !reflect.DeepEqual(got, []string{"zip -q -nw -d /x.zip -- dir/ dir/a dir/sub/ dir/sub/b"}) {
		t.Fatalf("commands = %q", got)
	}
}

// Thousands of members are deleted in several zip -d runs, each command
// line short enough for any platform to start.
func TestZipRemoveChunksLongLists(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"zip": true}}
	f.install(t)
	raws := make([]string, maxChunkNames*2+1)
	for i := range raws {
		raws[i] = fmt.Sprintf("dir/file-%04d", i)
	}
	if err := (zipBackend{}).remove(context.Background(), "/x.zip", raws); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("zip runs = %d, want 3", len(f.calls))
	}
	total := 0
	for _, c := range f.calls {
		total += len(c.args) - len([]string{"-q", "-nw", "-d", "/x.zip", "--"})
	}
	if total != len(raws) {
		t.Fatalf("names passed = %d, want %d", total, len(raws))
	}
}

func TestZipFallsBackToSevenZip(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"7za": true}}
	f.install(t)
	if err := (zipBackend{}).add(context.Background(), "/x.zip", "/stage", []string{"n"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := (zipBackend{}).remove(context.Background(), "/x.zip", []string{"d/", "d/a"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"7za a -tzip -y /x.zip -- n",
		"7za d -y /x.zip -- d/",
	}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
	if f.calls[0].dir != "/stage" {
		t.Errorf("7za a ran in %q, want /stage", f.calls[0].dir)
	}
}

// unzip only reads, and 7zr is the build of 7-Zip that knows 7z alone.
func TestZipWriteNeedsAWriter(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"unzip": true, "7zr": true}}
	f.install(t)
	b := zipBackend{}
	if err := b.checkWrite(context.Background(), "/x.zip", writeAdd, "m"); !errors.Is(err, errNoZipWriter) {
		t.Errorf("checkWrite = %v, want errNoZipWriter", err)
	}
	if err := b.add(context.Background(), "/x.zip", "/s", []string{"m"}, nil); !errors.Is(err, errNoZipWriter) {
		t.Errorf("add = %v, want errNoZipWriter", err)
	}
	if err := b.remove(context.Background(), "/x.zip", []string{"m"}); !errors.Is(err, errNoZipWriter) {
		t.Errorf("remove = %v, want errNoZipWriter", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("commands ran: %q", f.commands())
	}
}
