package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type findFileFinderProbeVFS struct {
	vfs.VFS
	called bool
}

func (v *findFileFinderProbeVFS) FindFiles(context.Context, string, vfs.FindQuery) ([]vfs.FoundEntry, error) {
	v.called = true
	return nil, nil
}

func TestSplitFindMasks(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		wantIncludes []string
		wantExcludes []string
	}{
		{name: "ordinary masks", input: " *.go, *.txt ", wantIncludes: []string{"*.go", "*.txt"}},
		{name: "included and excluded", input: "*.txt | .git, skip.txt", wantIncludes: []string{"*.txt"}, wantExcludes: []string{".git", "skip.txt"}},
		{name: "empty include defaults to all", input: " | .git", wantIncludes: []string{"*"}, wantExcludes: []string{".git"}},
		{name: "far star dot star", input: "*.* | *.tmp", wantIncludes: []string{"*"}, wantExcludes: []string{"*.tmp"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			includes, excludes := splitFindMasks(tc.input)
			if strings.Join(includes, ",") != strings.Join(tc.wantIncludes, ",") {
				t.Fatalf("includes = %#v, want %#v", includes, tc.wantIncludes)
			}
			if strings.Join(excludes, ",") != strings.Join(tc.wantExcludes, ",") {
				t.Fatalf("excludes = %#v, want %#v", excludes, tc.wantExcludes)
			}
		})
	}
}

func TestFindFileMaskMatches(t *testing.T) {
	if !findFileMaskMatches(".git", []string{"*.git", ".git"}) {
		t.Fatal("an excluded directory mask did not match")
	}
	if findFileMaskMatches("keep.txt", []string{".git", "skip.txt"}) {
		t.Fatal("an unrelated name matched an excluded mask")
	}
}

func TestFileContainsText_ChunkOverlap(t *testing.T) {
	// The word "SECRETPASSWORD" is 14 bytes long.
	// If chunk boundary splits it "SECRET" | "PASSWORD", we must still find it.

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "overlap.txt")

	// We will create a file just large enough to force our internal 128KB buffer to loop,
	// or we can test the function directly by overriding its internal buffer size logic
	// if it was exposed. Since it's hardcoded to 128KB, we write 128KB of padding,
	// then the secret word crossing the boundary.

	padding := make([]byte, 128*1024-6) // Leaves 6 bytes at the end of the first chunk
	for i := range padding {
		padding[i] = 'A'
	}

	data := append(padding, []byte("SECRETPASSWORD")...)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)

	// Test 1: Should find it
	found := fileContainsText(context.Background(), v, path, "secretpassword")
	if !found {
		t.Error("fileContainsText failed to find string crossing chunk boundary")
	}

	// Test 2: Should not find non-existent string
	foundMissing := fileContainsText(context.Background(), v, path, "missingpassword")
	if foundMissing {
		t.Error("fileContainsText falsely reported finding a non-existent string")
	}
}

func TestFindTextMatcherOptions(t *testing.T) {
	cases := []struct {
		name    string
		data    string
		pattern string
		options FindFileOptions
		want    bool
	}{
		{name: "case insensitive", data: "Needle", pattern: "needle", want: true},
		{name: "case sensitive miss", data: "Needle", pattern: "needle", options: FindFileOptions{CaseSensitive: true}, want: false},
		{name: "whole word miss", data: "needles", pattern: "needle", options: FindFileOptions{WholeWords: true}, want: false},
		{name: "whole word hit", data: "a needle here", pattern: "needle", options: FindFileOptions{WholeWords: true}, want: true},
		{name: "regexp", data: "file-42", pattern: `file-[0-9]+`, options: FindFileOptions{Regex: true, CaseSensitive: true}, want: true},
		{name: "regexp folded", data: "FILE-42", pattern: `file-[0-9]+`, options: FindFileOptions{Regex: true}, want: true},
		{name: "regexp case sensitive miss", data: "FILE-42", pattern: `file-[0-9]+`, options: FindFileOptions{Regex: true, CaseSensitive: true}, want: false},
		{name: "regexp whole word hit", data: "a file-42 here", pattern: `file-[0-9]+`, options: FindFileOptions{Regex: true, CaseSensitive: true, WholeWords: true}, want: true},
		{name: "regexp whole word miss", data: "prefile-42x", pattern: `file-[0-9]+`, options: FindFileOptions{Regex: true, CaseSensitive: true, WholeWords: true}, want: false},
		// Whole-word boundaries are decided per rune, so a multibyte
		// neighbour must be read as one letter and not as its first byte.
		{name: "cyrillic whole word hit", data: "вот иголка тут", pattern: "ИГОЛКА", options: FindFileOptions{WholeWords: true}, want: true},
		{name: "cyrillic whole word miss", data: "иголками", pattern: "иголка", options: FindFileOptions{WholeWords: true}, want: false},
		{name: "cyrillic regexp whole word miss", data: "иголками", pattern: "игол[кс]а", options: FindFileOptions{Regex: true, WholeWords: true}, want: false},
		{name: "not containing", data: "nothing here", pattern: "needle", options: FindFileOptions{NotContaining: true}, want: true},
		{name: "not containing miss", data: "needle here", pattern: "needle", options: FindFileOptions{NotContaining: true}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matcher, err := newFindTextMatcher(tc.pattern, tc.options)
			if err != nil {
				t.Fatal(err)
			}
			if got := matcher.matches([]byte(tc.data)); got != tc.want {
				t.Fatalf("matches(%q, %q) = %v, want %v", tc.data, tc.pattern, got, tc.want)
			}
		})
	}
}

func TestFindTextMatcherRejectsInvalidRegexp(t *testing.T) {
	if _, err := newFindTextMatcher("[", FindFileOptions{Regex: true}); err == nil {
		t.Fatal("invalid regexp was accepted")
	}
}

func TestExecuteFindFile_MaskMatching(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "test1.go"), []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test2.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "test3.go"), []byte("package test"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)

	ExecuteFindFile(nil, v, tmpDir, "*.go", "package", FindFileOptions{})

	// Drain UI tasks to wait for search completion
	timeout := time.After(2 * time.Second)
	isDone := false
	for !isDone {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			// If a message box appears, the search finished
			if vtui.FrameManager.GetTopFrameType() == vtui.TypeDialog {
				frame := vtui.FrameManager.GetTopFrame()
				// The title comes from the localization table, so comparing it
				// with an English literal makes this test depend on both the
				// active language and on how the INI parser trims the value.
				if frame != nil && frame.GetTitle() == Msg("FindFile.SearchResultsTitle") {
					isDone = true
					// Search successfully finished and showed the results dialog
				}
			}
		case <-timeout:
			t.Fatal("Search operation timed out")
		}
	}

	if !isDone {
		t.Error("Search did not complete successfully")
	}
}

func TestExecuteFindFile_ExcludesFilesAndDirectories(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "keep.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "skip.txt"), []byte("skip"), 0600); err != nil {
		t.Fatal(err)
	}
	excludedDir := filepath.Join(tmpDir, ".git")
	if err := os.Mkdir(excludedDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(excludedDir, "inside.txt"), []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}

	provider := &findFileFinderProbeVFS{VFS: vfs.NewOSVFS(tmpDir)}
	ExecuteFindFile(nil, provider, tmpDir, "*.txt | .git, skip.txt", "", FindFileOptions{})
	var results *SearchResultsWindow
	deadline := time.After(2 * time.Second)
	for results == nil {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if top, ok := vtui.FrameManager.GetTopFrame().(*SearchResultsWindow); ok {
				results = top
			}
		case <-deadline:
			t.Fatal("search operation timed out")
		}
	}
	defer results.Close()
	if len(results.found) != 1 || results.found[0].Item.Name != "keep.txt" {
		t.Fatalf("found = %#v, want only keep.txt", results.found)
	}
	if provider.called {
		t.Fatal("excluded-mask search must use the generic VFS walk")
	}
}

func TestLayout_SearchResultsDialog(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	v := vfs.NewOSVFS(t.TempDir())
	found := []FoundFile{{Path: filepath.FromSlash("/tmp/test.txt"), Item: vfs.VFSItem{Name: "test.txt", Size: 123}}}

	pf := NewPanelsFrame()
	defer pf.Close()
	ShowSearchResults(pf, v, found)

	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	vtui.AssertLayout(t, dlg)
}
