package archive

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestRemoveDirectoryPreservesOtherArchiveMembers(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "implicit", true: "explicit"}[explicit], func(t *testing.T) {
			dir := t.TempDir()
			archivePath := filepath.Join(dir, "fixture.zip")
			f, err := os.Create(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			writer := zip.NewWriter(f)
			names := []string{"folder/nested/item.txt", "folder-other/keep.txt", "keep.txt"}
			if explicit {
				names = append(names, "folder/", "folder/nested/", "folder/empty/")
			}
			for _, name := range names {
				entry, err := writer.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if name[len(name)-1] != '/' {
					if _, err := entry.Write([]byte(name)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
			archiveFS, err := NewArchiveVFS(vfs.NewOSVFS(dir), archivePath)
			if err != nil {
				t.Fatal(err)
			}
			defer archiveFS.Close()
			if err := archiveFS.Remove(context.Background(), archiveFS.Join(archiveFS.GetPath(), "folder")); err != nil {
				t.Fatal(err)
			}
			if _, err := archiveFS.Stat(context.Background(), archiveFS.Join(archiveFS.GetPath(), "folder")); !os.IsNotExist(err) {
				t.Fatalf("removed folder still present: %v", err)
			}
			reader, err := zip.OpenReader(archivePath)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			if len(reader.File) != 2 {
				t.Fatalf("remaining members: %v", len(reader.File))
			}
			for _, member := range reader.File {
				if member.Name != "keep.txt" && member.Name != "folder-other/keep.txt" {
					t.Fatalf("unexpected remaining member %s", member.Name)
				}
			}
		})
	}
}
