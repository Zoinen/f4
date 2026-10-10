package ap

// Undo (undo.go): the transaction journal a real Apply returns, and the
// rollback of a commit that fails halfway (docs/VTVIBE.md §7.4, f4#1606).

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"
)

// treeState is every path under dir (relative, "/"-separated) mapped to
// what is there: a directory, or a file with its exact bytes. Permission
// bits are part of it except on Windows, which only knows read-only. The
// patcher's own reports (afailed.md/afailed.ap) are left out - they are not
// part of the transaction.
func treeState(t *testing.T, dir string) map[string]string {
	t.Helper()
	// Read file contents through an os.Root scoped to dir rather than by
	// the WalkDir callback's own path: a plain os.ReadFile(p) here would
	// resolve p again from the filesystem root on every call, racing
	// against whatever WalkDir observed (gosec G122). root.ReadFile stays
	// confined to dir even if something under it is swapped for a symlink
	// between the WalkDir stat and this read.
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("open root %s: %v", dir, err)
	}
	defer func() { _ = root.Close() }()
	state := map[string]string{}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relOS, _ := filepath.Rel(dir, p)
		rel := filepath.ToSlash(relOS)
		if rel == "." || rel == "afailed.md" || rel == "afailed.ap" {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		perm := ""
		if runtime.GOOS != "windows" {
			perm = fmt.Sprintf(" %o", fi.Mode().Perm())
		}
		if d.IsDir() {
			state[rel] = "dir" + perm
			return nil
		}
		b, err := root.ReadFile(relOS)
		if err != nil {
			return err
		}
		state[rel] = "file" + perm + " " + fmt.Sprintf("%q", b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return state
}

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("setup %s: %v", name, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatalf("setup %s: %v", name, err)
		}
	}
}

func writePatch(t *testing.T, patch string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "p.ap")
	if err := os.WriteFile(p, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}
	return p
}

// TestUndoRevertsEveryKind applies a patch doing one of everything - an
// edit of a CRLF file with unusual permission bits, a RENAME into a new
// directory, a file DELETE, a whole-directory DELETE, a CREATE two missing
// directories deep and a directory CREATE - and reverts it: the tree must
// come back byte for byte, permission bits and emptied directories
// included.
func TestUndoRevertsEveryKind(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"a.txt":             "one\r\ntwo\r\nthree\r\n",
		"old.txt":           "to be moved\n",
		"gone.txt":          "to be deleted\n",
		"gonedir/x/y.txt":   "deep\n",
		"gonedir/top.txt":   "top\n",
		"keep/untouched.md": "keep me\n",
	})
	if err := os.Mkdir(filepath.Join(dir, "gonedir", "empty"), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.Chmod(filepath.Join(dir, "a.txt"), 0o640); err != nil {
		t.Fatalf("setup: %v", err)
	}
	patch := writePatch(t, "dd000001 AP 3.2\n\n"+
		"dd000001 FILE\na.txt\n\ndd000001 REPLACE\ndd000001 snippet\ntwo\ndd000001 content\nTWO\n\n"+
		"dd000001 FILE\nold.txt\n\ndd000001 RENAME\nmoved/new.txt\n\n"+
		"dd000001 FILE\ngone.txt\n\ndd000001 DELETE\n\n"+
		"dd000001 FILE\ngonedir\n\ndd000001 DELETE\n\n"+
		"dd000001 FILE\nmade/deep/file.txt\n\ndd000001 CREATE\ndd000001 content\nhello\n\n"+
		"dd000001 FILE\nemptydir/\n\ndd000001 CREATE\n")

	before := treeState(t, dir)
	res := Apply(patch, dir, Options{Silent: true})
	if res.Status != StatusSuccess {
		t.Fatalf("apply = %s %v, want SUCCESS", res.Status, res.Error)
	}
	if res.Undo == nil {
		t.Fatal("a real run that wrote files returned no Undo")
	}
	applied := treeState(t, dir)
	if applied["a.txt"] == before["a.txt"] || applied["moved/new.txt"] == "" || applied["gonedir"] != "" ||
		applied["made/deep/file.txt"] == "" || applied["emptydir"] == "" {
		t.Fatalf("the patch did not land as expected:\n%v", applied)
	}

	paths := res.Undo.Paths()
	sort.Strings(paths)
	want := []string{"a.txt", "emptydir", "gone.txt", "gonedir", "made", "moved", "old.txt"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("Undo.Paths() = %v, want %v (a new directory is recorded at its top, nothing nested)", paths, want)
	}
	if changed, err := res.Undo.Changed(); err != nil || len(changed) != 0 {
		t.Fatalf("Changed() right after apply = %v, %v; want none", changed, err)
	}

	if err := res.Undo.Revert(); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if got := treeState(t, dir); !reflect.DeepEqual(got, before) {
		t.Fatalf("tree after revert differs from before the patch:\n got %v\nwant %v", got, before)
	}
	if !res.Undo.Reverted() {
		t.Fatal("Reverted() = false after a successful Revert")
	}
	if err := res.Undo.Revert(); !errors.Is(err, ErrUndoReverted) {
		t.Fatalf("second Revert = %v, want ErrUndoReverted", err)
	}
}

// TestUndoRefusesAfterLaterChange: once anything the patch touched has
// changed since (edited, deleted), Revert refuses and restores nothing -
// not even the files nobody touched. Putting the paths back the way the
// patch left them makes the undo possible again.
func TestUndoRefusesAfterLaterChange(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "a1\na2\n", "b.txt": "b1\nb2\n"})
	patch := writePatch(t, "dd000002 AP 3.2\n\n"+
		"dd000002 FILE\na.txt\n\ndd000002 REPLACE\ndd000002 snippet\na1\ndd000002 content\nA1\n\n"+
		"dd000002 FILE\nb.txt\n\ndd000002 REPLACE\ndd000002 snippet\nb2\ndd000002 content\nB2\n\n"+
		"dd000002 FILE\nc.txt\n\ndd000002 CREATE\ndd000002 content\nnew\n")

	before := treeState(t, dir)
	res := Apply(patch, dir, Options{Silent: true})
	if res.Status != StatusSuccess || res.Undo == nil {
		t.Fatalf("apply = %s, undo %v; want SUCCESS with an Undo", res.Status, res.Undo)
	}
	patchedB := readBack(t, dir, "b.txt")
	patchedC := readBack(t, dir, "c.txt")

	// Someone edits b.txt after the patch.
	writeTree(t, dir, map[string]string{"b.txt": "edited by hand\n"})
	edited := treeState(t, dir)
	err := res.Undo.Revert()
	var conflict *UndoConflictError
	if !errors.As(err, &conflict) || !reflect.DeepEqual(conflict.Paths, []string{"b.txt"}) {
		t.Fatalf("Revert after an edit = %v, want UndoConflictError on [b.txt]", err)
	}
	if got := treeState(t, dir); !reflect.DeepEqual(got, edited) {
		t.Fatalf("a refused Revert changed the tree:\n got %v\nwant %v", got, edited)
	}

	// ... and deletes the file the patch created.
	if err := os.Remove(filepath.Join(dir, "c.txt")); err != nil {
		t.Fatalf("remove c.txt: %v", err)
	}
	if err := res.Undo.Revert(); !errors.As(err, &conflict) || !reflect.DeepEqual(conflict.Paths, []string{"b.txt", "c.txt"}) {
		t.Fatalf("Revert after an edit and a delete = %v, want UndoConflictError on [b.txt c.txt]", err)
	}
	if res.Undo.Reverted() {
		t.Fatal("a refused Revert marked the Undo as spent")
	}

	// Back to exactly what the patch left: the undo goes through.
	writeTree(t, dir, map[string]string{"b.txt": patchedB, "c.txt": patchedC})
	if err := res.Undo.Revert(); err != nil {
		t.Fatalf("Revert once the tree is as the patch left it: %v", err)
	}
	if got := treeState(t, dir); !reflect.DeepEqual(got, before) {
		t.Fatalf("tree after revert differs from before the patch:\n got %v\nwant %v", got, before)
	}
}

// TestUndoOnlyForRealWrites: no Undo when nothing was written - a dry run,
// a patch that is already applied, a failed one - and one for a partial
// run, which reverts only what that run wrote.
func TestUndoOnlyForRealWrites(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"a.txt": "one\ntwo\n"})
	patch := writePatch(t, "dd000003 AP 3.2\n\n"+
		"dd000003 FILE\na.txt\n\ndd000003 REPLACE\ndd000003 snippet\none\ndd000003 content\nONE\n")

	if res := Apply(patch, dir, Options{Silent: true, DryRun: true}); res.Undo != nil {
		t.Fatal("dry run returned an Undo")
	}
	if res := Apply(patch, dir, Options{Silent: true}); res.Status != StatusSuccess || res.Undo == nil {
		t.Fatalf("first real run = %s, undo %v; want SUCCESS with an Undo", res.Status, res.Undo)
	}
	if res := Apply(patch, dir, Options{Silent: true}); res.Status != StatusSuccess || res.Undo != nil {
		t.Fatalf("already-applied run = %s, undo %v; want SUCCESS and no Undo (nothing written)", res.Status, res.Undo)
	}
	missing := writePatch(t, "dd000004 AP 3.2\n\n"+
		"dd000004 FILE\nnope.txt\n\ndd000004 REPLACE\ndd000004 snippet\nx\ndd000004 content\ny\n")
	if res := Apply(missing, dir, Options{Silent: true, Strict: true}); res.Status != StatusFailed || res.Undo != nil {
		t.Fatalf("failed run = %s, undo %v; want FAILED and no Undo", res.Status, res.Undo)
	}

	dir2 := t.TempDir()
	writeTree(t, dir2, map[string]string{"a.txt": "one\ntwo\n"})
	partial := writePatch(t, "dd000005 AP 3.2\n\n"+
		"dd000005 FILE\na.txt\n\n"+
		"dd000005 REPLACE\ndd000005 snippet\none\ndd000005 content\nONE\n\n"+
		"dd000005 REPLACE\ndd000005 snippet\nnot there\ndd000005 content\nX\n")
	before := treeState(t, dir2)
	res := Apply(partial, dir2, Options{Silent: true})
	if res.Status != StatusPartial || res.Undo == nil {
		t.Fatalf("partial run = %s, undo %v; want PARTIAL with an Undo", res.Status, res.Undo)
	}
	if err := res.Undo.Revert(); err != nil {
		t.Fatalf("Revert of a partial run: %v", err)
	}
	if got := treeState(t, dir2); !reflect.DeepEqual(got, before) {
		t.Fatalf("tree after reverting a partial run:\n got %v\nwant %v", got, before)
	}
}

// TestCommitFailureRollsBack: a write that fails in the middle of the
// commit (CREATE of a file where a directory stands passes planning and
// fails only at the write) rolls back what the commit already wrote, so a
// failed patch leaves the tree as it found it (§7.4 step 3).
func TestCommitFailureRollsBack(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprintf("strict=%v", strict), func(t *testing.T) {
			dir := t.TempDir()
			writeTree(t, dir, map[string]string{"a.txt": "one\ntwo\n", "adir/inner.txt": "inside\n"})
			patch := writePatch(t, "dd000006 AP 3.2\n\n"+
				"dd000006 FILE\na.txt\n\ndd000006 REPLACE\ndd000006 snippet\none\ndd000006 content\nONE\n\n"+
				"dd000006 FILE\nnewdir/b.txt\n\ndd000006 CREATE\ndd000006 content\nb\n\n"+
				"dd000006 FILE\nadir\n\ndd000006 CREATE\ndd000006 content\nnot a directory\n")
			before := treeState(t, dir)
			res := Apply(patch, dir, Options{Silent: true, Strict: strict})
			if res.Status != StatusFailed || res.Error == nil || res.Error.Code != ErrFileWriteError {
				t.Fatalf("apply = %s %v, want FAILED with %s", res.Status, res.Error, ErrFileWriteError)
			}
			if res.Undo != nil {
				t.Fatal("a failed commit returned an Undo")
			}
			if got := treeState(t, dir); !reflect.DeepEqual(got, before) {
				t.Fatalf("tree after a failed commit:\n got %v\nwant %v", got, before)
			}
			t.Logf("error: %s", res.Error.Message)
		})
	}
}

func TestDisjointRoots(t *testing.T) {
	sep := string(filepath.Separator)
	in := []string{"a", "b" + sep + "c", "b", "a", "ab", "b" + sep + "d" + sep + "e"}
	if got, want := disjointRoots(in), []string{"a", "b", "ab"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("disjointRoots(%v) = %v, want %v", in, got, want)
	}
}
