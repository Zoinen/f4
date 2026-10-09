package multiarc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func tarRealFixture(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"top.txt":       "top",
		"dir/a.txt":     "a",
		"dir/[ab].txt":  "brackets",
		"dir/sub/b.txt": "b",
	})
	return src
}

// GNU tar: add, replace, mkdir and delete on a tarball built with "tar -cf
// x.tar ." -- every member under "./", the case that needs raw names -- in
// both a plain and a gzip-compressed one.
func TestTarRealWriteGNU(t *testing.T) {
	if realTarFlavor(t) != tarGNU {
		t.Skip("the tar on PATH is not GNU tar")
	}
	for _, name := range []string{"x.tar", "x.tar.gz"} {
		t.Run(name, func(t *testing.T) {
			if strings.HasSuffix(name, ".gz") {
				requireRealTool(t, "gzip")
			}
			ctx := context.Background()
			src := tarRealFixture(t)
			arc := filepath.Join(t.TempDir(), name)
			// --force-local: arc's own absolute path, a Windows temp
			// directory when this runs there, would otherwise be read by
			// GNU tar as a "host:path" remote-archive spec.
			if strings.HasSuffix(name, ".gz") {
				runReal(t, src, "tar", "--force-local", "-czf", arc, ".")
			} else {
				runReal(t, src, "tar", "--force-local", "-cf", arc, ".")
			}
			t.Cleanup(closeSharedMultiArcTempDirs)
			v := openReal(t, arc)

			writeMember(t, v, "/dir/new.txt", "new")
			writeMember(t, v, "/top.txt", "replaced")
			if err := v.MkDir(ctx, "/empty"); err != nil {
				t.Fatalf("MkDir: %v", err)
			}
			// Literal: with wildcards on, "[ab].txt" would take a.txt along.
			if err := v.Remove(ctx, "/dir/[ab].txt"); err != nil {
				t.Fatalf("Remove [ab].txt: %v", err)
			}
			if err := v.Remove(ctx, "/dir/sub"); err != nil {
				t.Fatalf("Remove dir/sub: %v", err)
			}
			// top.txt once: the replaced copy was deleted, not left behind.
			assertMembers(t, arc, "dir/", "dir/a.txt", "dir/new.txt", "empty/", "top.txt")
			if got := readMember(t, v, "/top.txt"); got != "replaced" {
				t.Errorf("top.txt = %q, want replaced", got)
			}
			if got := readMember(t, v, "/dir/a.txt"); got != "a" {
				t.Errorf("dir/a.txt = %q, want the untouched original", got)
			}
			assertNoScratchLeft(t, arc)
		})
	}
}

// useRealBSDTar makes sure "tar" on PATH is bsdtar for the rest of the
// test: it already is on macOS and Windows; elsewhere bsdtar, when
// installed, is put on PATH under that name.
func useRealBSDTar(t *testing.T) {
	t.Helper()
	if realTarFlavor(t) == tarBSD {
		return
	}
	pathOnly(t, map[string]string{"tar": "bsdtar"})
	if probeTar(context.Background()).flavor != tarBSD {
		t.Skip("could not put bsdtar on PATH as tar")
	}
}

// bsdtar appends, to a compressed tarball too, and refuses what it cannot
// do safely -- delete and replace -- with the reason, leaving the archive
// as it was.
func TestTarRealWriteBSD(t *testing.T) {
	useRealBSDTar(t)
	for _, name := range []string{"x.tar", "x.tar.gz"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			src := tarRealFixture(t)
			arc := filepath.Join(t.TempDir(), name)
			runReal(t, src, "tar", "-a", "-cf", arc, "top.txt", "dir")
			t.Cleanup(closeSharedMultiArcTempDirs)
			v := openReal(t, arc)

			writeMember(t, v, "/dir/new.txt", "new")
			if err := v.MkDir(ctx, "/empty"); err != nil {
				t.Fatalf("MkDir: %v", err)
			}
			writeMember(t, v, "/@at.txt", "at")
			if _, err := v.Create(ctx, "/top.txt"); err == nil || !strings.Contains(err.Error(), "bsdtar cannot replace") {
				t.Errorf("Create over top.txt = %v, want bsdtar's refusal", err)
			}
			if err := v.Remove(ctx, "/dir/a.txt"); err == nil || !strings.Contains(err.Error(), "bsdtar cannot delete") {
				t.Errorf("Remove = %v, want bsdtar's refusal", err)
			}
			assertMembers(t, arc, "@at.txt", "dir/", "dir/[ab].txt", "dir/a.txt", "dir/new.txt", "dir/sub/", "dir/sub/b.txt", "empty/", "top.txt")
			if got := readMember(t, v, "/dir/sub/b.txt"); got != "b" {
				t.Errorf("dir/sub/b.txt = %q: an existing member must survive the rewrite", got)
			}
			if got := readMember(t, v, "/@at.txt"); got != "at" {
				t.Errorf("@at.txt = %q", got)
			}
			assertNoScratchLeft(t, arc)
		})
	}
}

// "tar -cf x.tar -C dir ." stores every member under "./", and GNU tar will
// not extract "./dir/file.txt" when asked for "dir/file.txt". Viewing such a
// member used to fail with "Not found in archive"; Open now asks for the
// name the listing gave.
func TestTarRealOpensDotSlashMember(t *testing.T) {
	requireRealTool(t, "tar")
	src := t.TempDir()
	writeTree(t, src, map[string]string{"dir/file.txt": "hello"})
	arc := filepath.Join(t.TempDir(), "dot.tar")
	args := []string{"-cf", arc, "."}
	// --force-local: see TestTarRealWriteGNU.
	if realTarFlavor(t) == tarGNU {
		args = append([]string{"--force-local"}, args...)
	}
	runReal(t, src, "tar", args...)
	t.Cleanup(closeSharedMultiArcTempDirs)

	v := openReal(t, arc)
	if got := readMember(t, v, "/dir/file.txt"); got != "hello" {
		t.Fatalf("member content = %q, want hello", got)
	}
}
