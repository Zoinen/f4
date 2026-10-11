package multiarc

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// 7z a runs in the staging directory, so the member keeps the relative path
// it was staged at; "--" keeps a name like "-x" a name (an "@" name needs
// more than "--", see TestSevenZipAtNameGetsDotSlash).
func TestSevenZipAddRunsInStageDir(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"7zr": true}}
	f.install(t)
	if err := (sevenZipBackend{}).add(context.Background(), "/a/x.7z", "/stage", []string{"dir/-new.txt"}, nil); err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("calls = %#v, want one", f.calls)
	}
	c := f.calls[0]
	want := []string{"a", "-t7z", "-y", "/a/x.7z", "--", "dir/-new.txt"}
	if c.name != "7zr" || c.dir != "/stage" || !reflect.DeepEqual(c.args, want) {
		t.Fatalf("call = %s in %q %v, want 7zr in /stage %v", c.name, c.dir, c.args, want)
	}
}

func TestSevenZipWildcardNamesGetSpd(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"7z": true}}
	f.install(t)
	if err := (sevenZipBackend{bin: "7z"}).add(context.Background(), "/x.7z", "/s", []string{"w*ld"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := (sevenZipBackend{bin: "7z"}).remove(context.Background(), "/x.7z", []string{"a?"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"7z a -t7z -y -spd /x.7z -- w*ld",
		"7z d -y -spd /x.7z -- a?",
	}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

// 7-Zip reads an "@" argument as a listfile to read further names from, on
// "a" and "d" alike, wherever it falls among the file names -- "--" only
// stops switch parsing, not this -- so a member actually called "@odd.txt"
// goes in as "./@odd.txt" instead.
func TestSevenZipAtNameGetsDotSlash(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"7z": true}}
	f.install(t)
	if err := (sevenZipBackend{bin: "7z"}).add(context.Background(), "/x.7z", "/s", []string{"@odd.txt"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := (sevenZipBackend{bin: "7z"}).remove(context.Background(), "/x.7z", []string{"@odd.txt"}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"7z a -t7z -y /x.7z -- ./@odd.txt",
		"7z d -y /x.7z -- ./@odd.txt",
	}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

// 7z d deletes a directory with everything under it, so only the covering
// names are passed.
func TestSevenZipRemoveUsesCoveringNames(t *testing.T) {
	f := &fakeArchiver{tools: map[string]bool{"7za": true}}
	f.install(t)
	if err := (sevenZipBackend{}).remove(context.Background(), "/x.7z", []string{"dir", "dir/a", "dir/b/c", "top"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if got := f.commands(); !reflect.DeepEqual(got, []string{"7za d -y /x.7z -- dir top"}) {
		t.Fatalf("commands = %q", got)
	}
}

func TestSevenZipWriteWithoutToolIsRefused(t *testing.T) {
	f := &fakeArchiver{}
	f.install(t)
	b := sevenZipBackend{}
	if err := b.checkWrite(context.Background(), "/x.7z", writeAdd, "m"); !errors.Is(err, errNoSevenZip) {
		t.Errorf("checkWrite = %v, want errNoSevenZip", err)
	}
	if err := b.add(context.Background(), "/x.7z", "/s", []string{"m"}, nil); !errors.Is(err, errNoSevenZip) {
		t.Errorf("add = %v, want errNoSevenZip", err)
	}
	if err := b.remove(context.Background(), "/x.7z", []string{"m"}); !errors.Is(err, errNoSevenZip) {
		t.Errorf("remove = %v, want errNoSevenZip", err)
	}
}
