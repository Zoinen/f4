package multiarc

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// normalizeWorkDir replaces the random work directory a command's absolute
// -f path was built from with "<work>", the way placeholders does for
// create_test.go's own commands.
func normalizeWorkDir(cmds []string) []string {
	out := make([]string, len(cmds))
	for i, c := range cmds {
		out[i] = workDirPattern.ReplaceAllString(c, "<work>")
	}
	return out
}

const (
	gnuTarVersion     = "tar (GNU tar) 1.35\nCopyright (C) 2023 Free Software Foundation, Inc.\n"
	bsdTarVersion     = "bsdtar 3.7.2 - libarchive 3.7.2 zlib/1.3 liblzma/5.4.5 bz2lib/1.0.8 libzstd/1.5.5\n"
	winTarVersion     = "bsdtar 3.5.2 - libarchive 3.5.2 zlib/1.2.5.f-ipp bz2lib/1.0.6\n"
	busyBoxTarVersion = "tar: unrecognized option '--version'\nBusyBox v1.36.1 (2024-01-01) multi-call binary.\n\nUsage: tar c|x|t [-zjJvO] ...\n"
)

func TestProbeTarFlavors(t *testing.T) {
	cases := []struct {
		stdout, stderr string
		want           tarFlavor
		name           string
	}{
		{gnuTarVersion, "", tarGNU, "GNU tar"},
		{bsdTarVersion, "", tarBSD, "bsdtar"},
		{winTarVersion, "", tarBSD, "bsdtar"},
		{"", busyBoxTarVersion, tarBusyBox, "BusyBox tar"},
		{"", "tar: unknown option -- -\nusage: tar {crtux}[014578befHhjLmNOoPpqsvwXZz]\n", tarOther, "the tar on PATH"},
	}
	for _, c := range cases {
		f := &fakeArchiver{tools: map[string]bool{"tar": true}, tarVersion: c.stdout, tarVersionErr: c.stderr}
		f.install(t)
		info := probeTar(context.Background())
		if info.flavor != c.want || info.name() != c.name {
			t.Errorf("probeTar(%q, %q) = %v (%s), want %v (%s)", c.stdout, c.stderr, info.flavor, info.name(), c.want, c.name)
		}
	}
}

func TestTarCompressionFor(t *testing.T) {
	cases := map[string]string{
		"a.tar": "", "a.TAR": "",
		"a.tar.gz": "gzip", "a.tgz": "gzip",
		"a.tar.bz2": "bzip2", "a.tbz2": "bzip2", "a.tbz": "bzip2", "a.tb2": "bzip2",
		"a.tar.xz": "xz", "a.txz": "xz",
		"a.tar.zst": "zstd", "a.tzst": "zstd",
		"a.tar.lz": "lzip", "a.tlz": "lzip",
	}
	for name, tool := range cases {
		comp, plain, ok := tarCompressionFor(name)
		if !ok || plain != (tool == "") || comp.tool != tool {
			t.Errorf("tarCompressionFor(%q) = (%q, plain=%v, ok=%v), want tool %q", name, comp.tool, plain, ok, tool)
		}
	}
	for _, name := range []string{"a.taz", "a.tar.Z", "a.zip"} {
		if _, _, ok := tarCompressionFor(name); ok {
			t.Errorf("tarCompressionFor(%q) should not be writable", name)
		}
	}
}

// TestPlanTarEditRefusals is the "clear error" half of the tar matrix: each
// case is something the tools on PATH cannot do, and the error has to say
// what is missing rather than fail inside a half-run command.
func TestPlanTarEditRefusals(t *testing.T) {
	cases := []struct {
		name    string
		archive string
		tools   []string
		version string
		stderr  string
		op      writeOp
		want    string
	}{
		{"GNU tar without the compressor", "a.tar.xz", []string{"tar"}, gnuTarVersion, "", writeAdd, "needs xz on PATH"},
		{"bsdtar cannot delete", "a.tar", []string{"tar"}, bsdTarVersion, "", writeRemove, "bsdtar cannot delete"},
		{"bsdtar cannot replace", "a.tar.gz", []string{"tar"}, bsdTarVersion, "", writeReplace, "bsdtar cannot replace"},
		{"Windows bsdtar has no xz", "a.tar.xz", []string{"tar"}, winTarVersion, "", writeAdd, "needs xz, which this bsdtar lacks"},
		{"BusyBox tar", "a.tar", []string{"tar"}, "", busyBoxTarVersion, writeAdd, "BusyBox tar cannot change an existing archive"},
		{"unknown tar", "a.tar", []string{"tar"}, "", "usage: tar", writeRemove, "the tar on PATH cannot change"},
		{"compress(1) tarball", "a.taz", []string{"tar", "gzip"}, gnuTarVersion, "", writeAdd, "a.taz cannot be changed"},
	}
	for _, c := range cases {
		tools := map[string]bool{}
		for _, tool := range c.tools {
			tools[tool] = true
		}
		f := &fakeArchiver{tools: tools, tarVersion: c.version, tarVersionErr: c.stderr}
		f.install(t)
		err := tarBackend{}.checkWrite(context.Background(), "/x/"+c.archive, c.op, "m")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: checkWrite = %v, want an error containing %q", c.name, err, c.want)
		}
	}
}

func TestPlanTarEditAccepts(t *testing.T) {
	cases := []struct {
		name    string
		archive string
		tools   []string
		version string
		op      writeOp
	}{
		{"GNU tar deletes from a plain tar", "a.tar", []string{"tar"}, gnuTarVersion, writeRemove},
		{"GNU tar with gzip replaces in a tgz", "a.tgz", []string{"tar", "gzip"}, gnuTarVersion, writeReplace},
		{"bsdtar appends to a plain tar", "a.tar", []string{"tar"}, bsdTarVersion, writeAdd},
		{"bsdtar with built-in zlib appends to a tar.gz", "a.tar.gz", []string{"tar"}, winTarVersion, writeMkDir},
		{"bsdtar without liblzma falls back to xz on PATH", "a.tar.xz", []string{"tar", "xz"}, winTarVersion, writeAdd},
	}
	for _, c := range cases {
		tools := map[string]bool{}
		for _, tool := range c.tools {
			tools[tool] = true
		}
		f := &fakeArchiver{tools: tools, tarVersion: c.version}
		f.install(t)
		if err := (tarBackend{}).checkWrite(context.Background(), "/x/"+c.archive, c.op, "m"); err != nil {
			t.Errorf("%s: checkWrite = %v, want nil", c.name, err)
		}
	}
}

// GNU tar cannot append to a compressed tarball, so the member goes into a
// decompressed private copy that is compressed again and renamed over the
// original.
func TestTarAddGNUCompressedEditsPrivateCopy(t *testing.T) {
	arc := fakeArchive(t, "a.tar.gz", "ORIG")
	f := &fakeArchiver{tools: map[string]bool{"tar": true, "gzip": true}, tarVersion: gnuTarVersion}
	f.install(t)

	if err := (tarBackend{}).add(context.Background(), arc, "/stage", []string{"dir/new.txt"}, nil); err != nil {
		t.Fatalf("add: %v", err)
	}
	// The "-r" itself runs in stageDir, not workDir (see the comment on
	// add), so the archive it appends to is named by its absolute path.
	want := []string{
		"gzip -d -f work.tar.gz",
		"tar -r --force-local -f <work>" + string(filepath.Separator) + "work.tar -- dir/new.txt",
		"gzip -f work.tar",
	}
	if got := normalizeWorkDir(f.commands()); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
	// calls[0] is planTarEdit's own "tar --version" probe, in f4's own
	// directory; calls[1] is "gzip -d", calls[2] the "-r" and calls[3]
	// "gzip -f".
	if f.calls[2].dir != "/stage" {
		t.Errorf("tar -r ran in %q, want /stage", f.calls[2].dir)
	}
	for _, c := range []fakeCall{f.calls[1], f.calls[3]} {
		if c.dir == "" || !strings.HasPrefix(c.dir, strings.TrimSuffix(arc, "a.tar.gz")) {
			t.Errorf("%s ran in %q, want the work directory next to the archive", c.name, c.dir)
		}
	}
	if got := readArchive(t, arc); got != "ORIG|r:dir/new.txt" {
		t.Fatalf("archive = %q, want the edited copy", got)
	}
	assertNoScratchLeft(t, arc)
}

// Replacing with GNU tar deletes the old member by its raw name first:
// "tar -r" alone would leave both copies in the archive.
func TestTarReplaceGNUDeletesOldMemberFirst(t *testing.T) {
	arc := fakeArchive(t, "a.tar", "ORIG")
	f := &fakeArchiver{tools: map[string]bool{"tar": true}, tarVersion: gnuTarVersion}
	f.install(t)

	if err := (tarBackend{}).add(context.Background(), arc, "/stage", []string{"dir/f.txt"}, []string{"./dir/f.txt"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	want := []string{
		"tar --delete --no-wildcards -f work.tar -- ./dir/f.txt",
		"tar -r --force-local -f <work>" + string(filepath.Separator) + "work.tar -- dir/f.txt",
	}
	if got := normalizeWorkDir(f.commands()); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
	if got := readArchive(t, arc); got != "ORIG|d:./dir/f.txt|r:dir/f.txt" {
		t.Fatalf("archive = %q", got)
	}
}

func TestTarRemoveGNUPassesCoveringNames(t *testing.T) {
	arc := fakeArchive(t, "a.tar.bz2", "ORIG")
	f := &fakeArchiver{tools: map[string]bool{"tar": true, "bzip2": true}, tarVersion: gnuTarVersion}
	f.install(t)

	if err := (tarBackend{}).remove(context.Background(), arc, []string{"dir/", "dir/a", "dir/sub/b", "other"}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	want := []string{
		"bzip2 -d -f work.tar.bz2",
		"tar --delete --no-wildcards -f work.tar -- dir/ other",
		"bzip2 -f work.tar",
	}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
	if got := readArchive(t, arc); got != "ORIG|d:dir/,other" {
		t.Fatalf("archive = %q", got)
	}
}

// bsdtar appends to a compressed tarball by writing a new one that copies
// the old one's entries ("@archive") and adds the member after them. A name
// starting with "@" would itself be read as an archive, so it goes in as
// "./@name".
func TestTarAddBSDCompressedCopiesEntries(t *testing.T) {
	arc := fakeArchive(t, "a.tgz", "ORIG")
	f := &fakeArchiver{tools: map[string]bool{"tar": true}, tarVersion: bsdTarVersion}
	f.install(t)

	if err := (tarBackend{}).add(context.Background(), arc, "/stage", []string{"@odd.txt"}, nil); err != nil {
		t.Fatalf("add: %v", err)
	}
	want := []string{"tar -c -z -f work.tar.gz -C /stage -- @" + arc + " ./@odd.txt"}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
	if got := readArchive(t, arc); got != "ORIG|c:./@odd.txt" {
		t.Fatalf("archive = %q", got)
	}
	assertNoScratchLeft(t, arc)
}

func TestTarAddBSDPlainAppends(t *testing.T) {
	arc := fakeArchive(t, "a.tar", "ORIG")
	f := &fakeArchiver{tools: map[string]bool{"tar": true}, tarVersion: bsdTarVersion}
	f.install(t)

	if err := (tarBackend{}).add(context.Background(), arc, "/stage", []string{"new"}, nil); err != nil {
		t.Fatalf("add: %v", err)
	}
	want := []string{"tar -r -f <work>" + string(filepath.Separator) + "work.tar -- new"}
	if got := normalizeWorkDir(f.commands()); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
	// calls[0] is planTarEdit's own "tar --version" probe; calls[1] is the
	// "-r" itself.
	if f.calls[1].dir != "/stage" {
		t.Errorf("tar -r ran in %q, want /stage", f.calls[1].dir)
	}
	if got := readArchive(t, arc); got != "ORIG|r:new" {
		t.Fatalf("archive = %q", got)
	}
}

// Any step failing leaves the original exactly as it was, and its error
// carries what the tool said.
func TestTarEditFailureLeavesArchiveUntouched(t *testing.T) {
	for _, failing := range []string{"gzip -d", "tar -r", "gzip -f"} {
		arc := fakeArchive(t, "a.tar.gz", "ORIG")
		f := &fakeArchiver{
			tools:      map[string]bool{"tar": true, "gzip": true},
			tarVersion: gnuTarVersion,
			fail:       map[string]error{failing: errors.New("exit status 2")},
		}
		f.install(t)
		err := (tarBackend{}).add(context.Background(), arc, "/stage", []string{"n"}, nil)
		tool := strings.Fields(failing)[0]
		if err == nil || !strings.Contains(err.Error(), tool+" says no") {
			t.Errorf("%s failing: add error = %v, want the tool's own message", failing, err)
		}
		if got := readArchive(t, arc); got != "ORIG" {
			t.Errorf("%s failing: archive = %q, want it untouched", failing, got)
		}
		assertNoScratchLeft(t, arc)
	}
}

func TestTarRemoveRefusedByBSDRunsNothing(t *testing.T) {
	arc := fakeArchive(t, "a.tar", "ORIG")
	f := &fakeArchiver{tools: map[string]bool{"tar": true}, tarVersion: bsdTarVersion}
	f.install(t)
	if err := (tarBackend{}).remove(context.Background(), arc, []string{"x"}); err == nil {
		t.Fatal("bsdtar remove should be refused")
	}
	if got := f.commands(); len(got) != 0 {
		t.Fatalf("commands = %q, want none", got)
	}
}
