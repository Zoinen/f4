package intchecker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// makeTree creates files (slash-separated names, relative to dir) holding
// "abc", with their directories.
func makeTree(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("abc"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// storedNames lists the names a checksum file holds.
func storedNames(t *testing.T, name string, data string) []string {
	t.Helper()
	file, err := ParseHashFile(name, []byte(data))
	if err != nil {
		t.Fatalf("%s: %v\n%s", name, err, data)
	}
	var names []string
	for _, e := range file.Entries {
		names = append(names, e.Name)
	}
	return names
}

func TestMatchesMask(t *testing.T) {
	for _, tc := range []struct {
		mask, name string
		want       bool
	}{
		{"", "a.bak", true},
		{"*", "sub/a.bak", true},
		{"*.*", "noext", true},
		{"*.txt", "a.txt", true},
		{"*.txt", "sub/deep/a.txt", true},
		{"*.txt", "a.bak", false},
		{"*.JPG", "sub/photo.jpg", true},
		{"*.jpg,*.png", "b.png", true},
		{"*.jpg;*.png", "b.gif", false},
		{"*|*.bak", "a.txt", true},
		{"*|*.bak", "sub/a.bak", false},
		{"|*.bak", "a.txt", true},
		{"|*.bak", "a.bak", false},
		{"sub*", "sub/x.txt", false}, // the mask sees the name, not the path
		{"/^[0-9]+\\.txt$/", "dir/42.txt", true},
	} {
		job := generateJob{mask: tc.mask}
		if got := job.matchesMask(tc.name); got != tc.want {
			t.Errorf("mask %q, %q: got %v, want %v", tc.mask, tc.name, got, tc.want)
		}
	}
}

func TestCollectInputsRecursesAndFilters(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "a.txt", "a.bak", "sub/b.txt", "sub/b.bak", "sub/deep/c.txt", "other/d.txt", "empty/.keep.bak")
	var scanned []string
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"sub", "a.txt", "a.bak", "empty"}, algorithm: AlgMD5, mode: outputSingle, output: "list.md5", recursive: true, mask: "*.txt"}
	var res generateResult
	inputs, err := collectInputs(context.Background(), job, &res, func(d string, files, dirs int64) {
		scanned = append(scanned, d)
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, in := range inputs {
		got = append(got, in.name)
		if in.size != 3 {
			t.Errorf("%s: size %d", in.name, in.size)
		}
	}
	if want := []string{"a.txt", "sub/b.txt", "sub/deep/c.txt"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("inputs = %q, want %q", got, want)
	}
	if len(res.Skipped) != 0 || len(res.Failures) != 0 {
		t.Fatalf("result = %+v", res)
	}
	sort.Strings(scanned)
	want := []string{filepath.Join(dir, "empty"), filepath.Join(dir, "sub"), filepath.Join(dir, "sub", "deep")}
	if strings.Join(scanned, "|") != strings.Join(want, "|") {
		t.Fatalf("scan progress = %q, want %q", scanned, want)
	}

	job.recursive = false
	res = generateResult{}
	inputs, err = collectInputs(context.Background(), job, &res, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0].name != "a.txt" || len(res.Skipped) != 2 {
		t.Fatalf("without recursion: inputs = %+v, result = %+v", inputs, res)
	}
}

func TestCollectInputsDoesNotFollowDirectoryLinks(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "sub/a.txt")
	if err := os.Symlink(dir, filepath.Join(dir, "sub", "loop")); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"sub"}, algorithm: AlgMD5, mode: outputDisplay, recursive: true}
	var res generateResult
	inputs, err := collectInputs(context.Background(), job, &res, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 1 || inputs[0].name != "sub/a.txt" || len(res.Skipped) != 1 || res.Skipped[0] != "sub/loop" {
		t.Fatalf("inputs = %+v, result = %+v", inputs, res)
	}
}

func TestCollectInputsCancelledWhileWalking(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "sub/a.txt", "sub/deep/b.txt")
	ctx, cancel := context.WithCancel(context.Background())
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"sub"}, algorithm: AlgMD5, mode: outputDisplay, recursive: true}
	var res generateResult
	_, err := collectInputs(ctx, job, &res, func(string, int64, int64) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestRunGenerateRecursiveMaskedSingleFile(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "a.txt", "a.bak", "sub/b.txt", "sub/b.bak", "sub/deep/c.txt")
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"sub", "a.bak", "a.txt"}, algorithm: AlgMD5, output: "list.md5", recursive: true, mask: "*|*.bak"}
	res, err := runGenerate(context.Background(), job, &recordingReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Written != 3 {
		t.Fatalf("result = %+v", res)
	}
	sum := abcDigests[AlgMD5]
	want := sum + " *a.txt\n" + sum + " *sub/b.txt\n" + sum + " *sub/deep/c.txt\n"
	if got := readTestFile(t, dir, "list.md5"); got != want {
		t.Fatalf("list.md5 =\n%s\nwant\n%s", got, want)
	}
}

func TestRunGenerateMaskMatchingNothing(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "a.txt", "sub/b.txt")
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"a.txt", "sub"}, algorithm: AlgMD5, output: "list.md5", recursive: true, mask: "*.jpg"}
	if _, err := runGenerate(context.Background(), job, &recordingReporter{}); !errors.Is(err, errNothingToHash) {
		t.Fatalf("err = %v, want errNothingToHash", err)
	}
}

func TestRunGenerateRecursiveDirectoryFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "photos")
	makeTree(t, dir, "a.txt", "sub/b.txt", "sub/deep/c.txt", "sub/deep/d.txt")
	// A list from an earlier run is replaced, not hashed into the new one.
	writeTestFile(t, filepath.Join(dir, "sub"), "sub.md5", "old")
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"a.txt", "sub"}, algorithm: AlgMD5, mode: outputDirectory, recursive: true, overwrite: true}
	res, err := runGenerate(context.Background(), job, &recordingReporter{})
	if err != nil {
		t.Fatal(err)
	}
	wantOutputs := []string{"photos.md5", "sub/deep/deep.md5", "sub/sub.md5"}
	if strings.Join(res.Outputs, "|") != strings.Join(wantOutputs, "|") || res.Written != 4 {
		t.Fatalf("result = %+v", res)
	}
	sum := abcDigests[AlgMD5]
	for name, want := range map[string]string{
		"photos.md5":        sum + " *a.txt\n",
		"sub/sub.md5":       sum + " *b.txt\n",
		"sub/deep/deep.md5": sum + " *c.txt\n" + sum + " *d.txt\n",
	} {
		if got := readTestFile(t, dir, filepath.FromSlash(name)); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestRunGenerateRecursiveSeparateFiles(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "sub/b.txt")
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"sub"}, algorithm: AlgCRC32, mode: outputSeparate, recursive: true}
	res, err := runGenerate(context.Background(), job, &recordingReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Outputs) != 1 || res.Outputs[0] != "sub/b.txt.sfv" {
		t.Fatalf("result = %+v", res)
	}
	if got, want := readTestFile(t, dir, filepath.Join("sub", "b.txt.sfv")), "; Generated by f4\nb.txt 352441C2\n"; got != want {
		t.Fatalf("b.txt.sfv = %q, want %q", got, want)
	}
}

func TestRunGenerateAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "a.txt", "sub/b.txt")
	fs := vfs.NewOSVFS(dir)
	abs := func(name string) string { return filepath.Join(dir, filepath.FromSlash(name)) }
	for _, tc := range []struct {
		mode   outputMode
		algo   Algorithm
		target string
		want   []string
	}{
		{outputSingle, AlgSHA256, "list.sha256", []string{abs("a.txt"), abs("sub/b.txt")}},
		{outputSingle, AlgCRC32, "list.sfv", []string{abs("a.txt"), abs("sub/b.txt")}},
		{outputSeparate, AlgMD5, "sub/b.txt.md5", []string{abs("sub/b.txt")}},
		{outputDirectory, AlgSHA1, "sub/sub.sha1", []string{abs("sub/b.txt")}},
	} {
		// Every round leaves checksum files in sub; the mask keeps them out
		// of the next one.
		job := generateJob{fs: fs, dir: dir, names: []string{"a.txt", "sub"}, algorithm: tc.algo, mode: tc.mode, output: tc.target, recursive: true, absolute: true, overwrite: true, mask: "*.txt"}
		if _, err := runGenerate(context.Background(), job, &recordingReporter{}); err != nil {
			t.Fatal(err)
		}
		got := storedNames(t, tc.target, readTestFile(t, dir, filepath.FromSlash(tc.target)))
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("mode %d %s: names = %q, want %q", tc.mode, tc.target, got, tc.want)
		}
	}

	job := generateJob{fs: fs, dir: dir, names: []string{"sub"}, algorithm: AlgMD5, mode: outputDisplay, recursive: true, absolute: true, mask: "*.txt"}
	res, err := runGenerate(context.Background(), job, &recordingReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if got := storedNames(t, "x.md5", res.Text); len(got) != 1 || got[0] != abs("sub/b.txt") {
		t.Fatalf("display names = %q", got)
	}
}

// TestGeneratedFilesValidate generates checksum files with every option and
// checks them the way "Validate files" does: parse, then look the listed
// names up from the checksum file's directory (or, for absolute paths, from
// an unrelated one).
func TestGeneratedFilesValidate(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "photos")
	makeTree(t, dir, "a.txt", "name with spaces.txt", "sub/b.txt", "sub/deep/c.txt")
	elsewhere := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	for _, tc := range []struct {
		label    string
		job      generateJob
		hashFile string // relative to dir
		checkIn  string // validation directory
		entries  int
	}{
		{"single relative", generateJob{mode: outputSingle, algorithm: AlgSHA256, output: "all.sha256"}, "all.sha256", dir, 4},
		{"single relative sfv", generateJob{mode: outputSingle, algorithm: AlgCRC32, output: "all.sfv"}, "all.sfv", dir, 4},
		{"single absolute", generateJob{mode: outputSingle, algorithm: AlgMD5, output: "abs.md5", absolute: true}, "abs.md5", elsewhere, 4},
		{"single absolute sfv", generateJob{mode: outputSingle, algorithm: AlgCRC32, output: "abs.sfv", absolute: true}, "abs.sfv", elsewhere, 4},
		{"per directory", generateJob{mode: outputDirectory, algorithm: AlgSHA1}, "sub/deep/deep.sha1", filepath.Join(dir, "sub", "deep"), 1},
		{"per directory top", generateJob{mode: outputDirectory, algorithm: AlgSHA512}, "photos.sha512", dir, 2},
		{"per file", generateJob{mode: outputSeparate, algorithm: AlgSHA384}, "sub/b.txt.sha384", filepath.Join(dir, "sub"), 1},
		{"per directory absolute", generateJob{mode: outputDirectory, algorithm: AlgMD5, absolute: true}, "sub/sub.md5", elsewhere, 1},
	} {
		job := tc.job
		job.fs, job.dir, job.recursive, job.overwrite = fs, dir, true, true
		job.mask = "*.txt" // keeps the checksum files of earlier rounds out
		job.names = []string{"a.txt", "name with spaces.txt", "sub"}
		if _, err := runGenerate(context.Background(), job, &recordingReporter{}); err != nil {
			t.Fatalf("%s: generate: %v", tc.label, err)
		}
		hashPath := filepath.Join(dir, filepath.FromSlash(tc.hashFile))
		data, err := os.ReadFile(hashPath)
		if err != nil {
			t.Fatalf("%s: %v", tc.label, err)
		}
		file, err := ParseHashFile(filepath.Base(hashPath), data)
		if err != nil {
			t.Fatalf("%s: parse: %v\n%s", tc.label, err, data)
		}
		if len(file.Entries) != tc.entries || file.Malformed != 0 || file.Algorithm != job.algorithm {
			t.Fatalf("%s: parsed %+v\n%s", tc.label, file, data)
		}
		res, err := runValidate(context.Background(), validateJob{fs: fs, hashPath: hashPath, dir: tc.checkIn, file: file}, &recordingReporter{})
		if err != nil {
			t.Fatalf("%s: validate: %v\n%s", tc.label, err, data)
		}
		if res.Counts[statusOK] != tc.entries {
			t.Fatalf("%s: validate = %+v\n%s", tc.label, res, data)
		}
	}

	// A file changed after hashing shows up as a mismatch in a subdirectory.
	if err := os.WriteFile(filepath.Join(dir, "sub", "deep", "c.txt"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := readTestFile(t, dir, "all.sha256")
	file, err := ParseHashFile("all.sha256", []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	res, err := runValidate(context.Background(), validateJob{fs: fs, dir: dir, file: file}, &recordingReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts[statusOK] != 3 || res.Counts[statusMismatch] != 1 {
		t.Fatalf("after change: %+v", res)
	}
	for _, r := range res.Results {
		if r.Status == statusMismatch && r.Name != "sub/deep/c.txt" {
			t.Fatalf("mismatch reported for %q", r.Name)
		}
	}
}

func TestGenerateDialogOptions(t *testing.T) {
	initValidateTestScreen(t)
	d := newGenerateDialog("photos", DefaultSettings())
	if d.recursive.State != 1 || d.absolute.State != 0 || d.editMask.GetText() != defaultMask {
		t.Fatalf("defaults: recursive=%d absolute=%d mask=%q", d.recursive.State, d.absolute.State, d.editMask.GetText())
	}
	_, y1, _, _ := d.editMask.GetPosition()
	_, ly, _, _ := d.btnOK.GetPosition()
	x1, _, x2, _ := d.editMask.GetPosition()
	wx1, wy1, wx2, wy2 := d.win.GetPosition()
	if y1 >= ly || x1 <= wx1 || x2 >= wx2 || y1 <= wy1 || ly >= wy2 {
		t.Fatalf("mask field at %d..%d row %d, OK row %d, window %d,%d-%d,%d", x1, x2, y1, ly, wx1, wy1, wx2, wy2)
	}
}
