package dialog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// buildRecursiveAttributesTree lays out a tree for the recursive-Set tests
// (f4#1502):
//
//	root/dirSel/top.txt
//	root/dirSel/real_sub/inner.txt
//	root/dirSel/link_to_external -> root/external_dir  (symlink, must be a leaf)
//	root/external_dir/leaked.txt
//
// link_to_external lets a test prove recursion never follows a symlink into
// its target: leaked.txt is reachable only by following that link, so it
// must stay untouched however the tree is walked.
func buildRecursiveAttributesTree(t *testing.T) (root, dirSel, topFile, realSub, innerFile, leakedFile string) {
	t.Helper()
	root = t.TempDir()
	dirSel = filepath.Join(root, "dirSel")
	realSub = filepath.Join(dirSel, "real_sub")
	external := filepath.Join(root, "external_dir")
	for _, dir := range []string{dirSel, realSub, external} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	topFile = filepath.Join(dirSel, "top.txt")
	innerFile = filepath.Join(realSub, "inner.txt")
	leakedFile = filepath.Join(external, "leaked.txt")
	for _, f := range []string{topFile, innerFile, leakedFile} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	if err := os.Symlink(external, filepath.Join(dirSel, "link_to_external")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return root, dirSel, topFile, realSub, innerFile, leakedFile
}

// showRecursiveAttributesDialog opens the Unix attributes dialog for a
// single selected directory and returns its recursive checkbox, Octal
// field and Set button, failing the test if any of them cannot be found.
func showRecursiveAttributesDialog(t *testing.T, v vfs.VFS, dirSel string) (dlg vtui.Container, cbRecursive *vtui.Checkbox, editOctal *vtui.Edit, setButton *vtui.Button) {
	t.Helper()
	item, err := vfs.Lstat(context.Background(), v, dirSel)
	if err != nil {
		t.Fatalf("lstat %s: %v", dirSel, err)
	}
	ShowAttributesUnixForTargets(nil, v, []AttributesTarget{{Path: dirSel, Item: item}})
	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(vtui.Container)
	if !ok {
		t.Fatal("attributes dialog not found")
	}
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		switch c := el.(type) {
		case *vtui.Checkbox:
			if strings.Contains(c.GetText(), "Subfolders") {
				cbRecursive = c
			}
		case *vtui.Edit:
			if c.Validator != nil {
				editOctal = c
			}
		case *vtui.Button:
			if strings.Contains(c.GetText(), i18n.Msg("Attributes.BtnSet")) {
				setButton = c
			}
		}
		return true
	})
	if cbRecursive == nil || editOctal == nil || setButton == nil {
		t.Fatal("recursive checkbox, octal field or Set button not found")
	}
	return dlg, cbRecursive, editOctal, setButton
}

func requirePosixModeBits(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not meaningful on GOOS=windows")
	}
}

// f4#1502: checking "Subfolders" and clicking Set must reach every file and
// subdirectory under the selected directory, not just the directory itself,
// while a symlink inside the tree stays a leaf -- its target's own contents
// must never be touched, matching how f4's own recursive delete
// (OSVFS.Remove -> os.RemoveAll) and QuickView's
// vfs.ScanOptions{FollowSymlinkDirs:false} already treat a symlink.
func TestAttributesDialog_RecursiveWalksWholeTreeButNotThroughSymlinks(t *testing.T) {
	requirePosixModeBits(t)

	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	root, dirSel, topFile, realSub, innerFile, leakedFile := buildRecursiveAttributesTree(t)
	v := vfs.NewOSVFS(root)

	dlg, cbRecursive, editOctal, setButton := showRecursiveAttributesDialog(t, v, dirSel)
	if cbRecursive.State != 0 {
		t.Fatalf("recursive checkbox state = %d, want 0 (unchecked by default)", cbRecursive.State)
	}
	cbRecursive.State = 1
	editOctal.SetText("0700")

	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	for _, path := range []string{dirSel, topFile, realSub, innerFile} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := st.Mode().Perm(); got != 0700 {
			t.Errorf("%s: mode = %04o, want 0700 (recursive Set must reach every entry)", path, got)
		}
	}

	// leaked.txt is reachable only by following link_to_external. If the
	// walk ever descended into a symlinked directory, this would be 0700 too.
	// It was created at 0o600 by buildRecursiveAttributesTree, so that's the
	// mode it must still have.
	if st, err := os.Stat(leakedFile); err != nil {
		t.Fatalf("stat %s: %v", leakedFile, err)
	} else if got := st.Mode().Perm(); got != 0o600 {
		t.Errorf("%s: mode = %04o, want untouched 0600 (recursion must not follow the symlink)", leakedFile, got)
	}

	// A recursive Set gets a completion summary (dirSel, top.txt, real_sub,
	// inner.txt, link_to_external = 5 objects) instead of leaving the user
	// looking at a closed dialog with no feedback at all.
	summary := fmt.Sprintf(i18n.Msg("Attributes.RecursiveApplied"), 5)
	found := false
	if top, ok := vtui.FrameManager.GetTopFrame().(vtui.UIElement); ok {
		walkUI(top, func(el vtui.UIElement) bool {
			if text, ok := el.(*vtui.Text); ok && strings.Contains(text.GetText(), summary) {
				found = true
			}
			return true
		})
	}
	if !found {
		t.Errorf("no completion summary %q found on top frame", summary)
	}
}

// Negative test (f4#1502): recursive must be opt-in. With the checkbox left
// at its default (unchecked) state, Set must behave exactly as it always
// has -- only the selected directory itself changes, its contents don't.
func TestAttributesDialog_RecursiveOffByDefaultLeavesTreeAlone(t *testing.T) {
	requirePosixModeBits(t)

	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	root, dirSel, topFile, realSub, innerFile, _ := buildRecursiveAttributesTree(t)
	v := vfs.NewOSVFS(root)

	dlg, cbRecursive, editOctal, setButton := showRecursiveAttributesDialog(t, v, dirSel)
	if cbRecursive.State != 0 {
		t.Fatalf("recursive checkbox state = %d, want 0 (unchecked by default)", cbRecursive.State)
	}
	// Deliberately not checking cbRecursive.
	editOctal.SetText("0700")

	setButton.OnClick()
	runUITasksUntil(t, fm.TaskChan, dlg.(vtui.Frame).IsDone)

	if st, err := os.Stat(dirSel); err != nil {
		t.Fatalf("stat %s: %v", dirSel, err)
	} else if got := st.Mode().Perm(); got != 0700 {
		t.Errorf("%s: mode = %04o, want 0700 (the selected object itself is always changed)", dirSel, got)
	}

	for _, path := range []string{topFile, realSub, innerFile} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := st.Mode().Perm(); got == 0700 {
			t.Errorf("%s: mode = %04o, want untouched -- recursive was not checked", path, got)
		}
	}
}

// The checkbox only makes sense for a selection that actually contains a
// real directory: offering it for a plain-file selection would promise a
// walk that has nothing to do.
func TestAttributesDialog_RecursiveCheckboxHiddenWithoutARealDirectory(t *testing.T) {
	requirePosixModeBits(t)

	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	root := t.TempDir()
	filePath := filepath.Join(root, "plain.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "link_to_dir")
	dirPath := filepath.Join(root, "some_dir")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dirPath, linkPath); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(root)
	fileItem, err := vfs.Lstat(context.Background(), v, filePath)
	if err != nil {
		t.Fatal(err)
	}
	// A symlink to a directory is still a symlink, not a "real" directory:
	// it must not make the checkbox appear either, since recursion would
	// never walk through it anyway.
	linkItem, err := vfs.Lstat(context.Background(), v, linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if !linkItem.IsSymlink || !linkItem.IsDir {
		t.Fatalf("test setup: want a symlink resolving to a directory, got IsSymlink=%v IsDir=%v", linkItem.IsSymlink, linkItem.IsDir)
	}

	ShowAttributesUnixForTargets(nil, v, []AttributesTarget{
		{Path: filePath, Item: fileItem},
		{Path: linkPath, Item: linkItem},
	})
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	var found bool
	walkUI(dlg.(vtui.UIElement), func(el vtui.UIElement) bool {
		if c, ok := el.(*vtui.Checkbox); ok && strings.Contains(c.GetText(), "Subfolders") {
			found = true
		}
		return true
	})
	if found {
		t.Error("recursive checkbox shown for a selection with no real directory")
	}
}

func TestTargetsIncludeRealDir(t *testing.T) {
	dir := AttributesTarget{Item: vfs.VFSItem{IsDir: true}}
	symlinkToDir := AttributesTarget{Item: vfs.VFSItem{IsDir: true, IsSymlink: true}}
	file := AttributesTarget{Item: vfs.VFSItem{}}

	if targetsIncludeRealDir(nil) {
		t.Error("empty selection must not report a real directory")
	}
	if targetsIncludeRealDir([]AttributesTarget{file, symlinkToDir}) {
		t.Error("a symlink resolving to a directory must not count as a real directory")
	}
	if !targetsIncludeRealDir([]AttributesTarget{file, dir}) {
		t.Error("a real directory in the selection must be detected")
	}
}
