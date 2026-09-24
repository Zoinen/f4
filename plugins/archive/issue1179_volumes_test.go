package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/archives"
	"github.com/unxed/f4/vfs"
)

// issue1179BuildSplitSevenZip writes a 7z archive split into three volumes the
// way 7-Zip's -v option does, a plain byte split, and returns the first one.
func issue1179BuildSplitSevenZip(t *testing.T, dir string) string {
	t.Helper()
	whole, _ := issue915BuildArchive(t, dir, "whole.7z", archives.SevenZip{}, 2, 64*1024)
	data, err := os.ReadFile(whole)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(whole); err != nil {
		t.Fatal(err)
	}
	third := len(data) / 3
	parts := [][]byte{data[:third], data[third : 2*third], data[2*third:]}
	for index, part := range parts {
		name := filepath.Join(dir, fmt.Sprintf("split.7z.%03d", index+1))
		if err := os.WriteFile(name, part, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "split.7z.001")
}

// Point 3 of issue #1179: entering a split 7z worked once zipper joined the
// volumes, but copying out of it (F5), testing (Shift-F3) and the post
// extraction check still read only the first volume.
func TestIssue1179SplitSevenZipCopiesTestsAndValidates(t *testing.T) {
	root := t.TempDir()
	first := issue1179BuildSplitSevenZip(t, root)
	ctx := context.Background()
	members := []string{"member00.bin", "member01.bin"}

	archiveVFS, err := NewArchiveVFSContext(ctx, vfs.NewOSVFS(root), filepath.Base(first))
	if err != nil {
		t.Fatalf("enter split archive: %v", err)
	}
	copied := t.TempDir()
	if err := archiveVFS.CopyBulkAt(ctx, archiveVFS.GetPath(), members, vfs.NewOSVFS(copied), copied, &issue915ProgressRecorder{}); err != nil {
		t.Fatalf("copy out of split archive: %v", err)
	}
	_ = archiveVFS.Close()
	for _, member := range members {
		if stat, err := os.Stat(filepath.Join(copied, member)); err != nil || stat.Size() != 64*1024 {
			t.Fatalf("copied %s: stat=%v err=%v", member, stat, err)
		}
	}

	if err := testArchiveWithPasswordPrompt(ctx, first, &issue915ProgressRecorder{}); err != nil {
		t.Fatalf("test split archive: %v", err)
	}

	extracted := t.TempDir()
	if err := extractArchiveWithPasswordPrompt(ctx, first, extracted, &issue915ProgressRecorder{}); err != nil {
		t.Fatalf("extract split archive: %v", err)
	}

	// The check after extraction must look at a split archive too: damage an
	// extracted member without changing its size and it has to notice.
	target := filepath.Join(extracted, members[0])
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 0xFF
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateExtracted7z(ctx, first, extracted, ""); err == nil {
		t.Fatal("validateExtracted7z() accepted a damaged member extracted from name.7z.001")
	}
}
