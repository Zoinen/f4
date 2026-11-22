package panel

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestAssociatedFileCommandEncodesTermuxPath(t *testing.T) {
	command, args, ok := AssociatedFileCommand("/storage/emulated/0/a b#100%.txt")
	want := []string{"file:///storage/emulated/0/a%20b%23100%25.txt"}
	if !ok || command != "termux-open" || !slices.Equal(args, want) {
		t.Fatalf("association = %q %q (%v), want termux-open %q", command, args, ok, want)
	}
}

func TestOpenAssociatedPathUsesAndroidDocumentsForSharedDirectory(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "storage")
	folder := filepath.Join(shared, "Download", "a b#1")
	if err := os.MkdirAll(folder, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked folder")
	if err := os.Symlink(folder, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EXTERNAL_STORAGE", shared)
	t.Setenv("PATH", dir)
	output := filepath.Join(dir, "args")
	t.Setenv("F4_TEST_ASSOCIATION_ARGS", output)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$F4_TEST_ASSOCIATION_ARGS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "am"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := openAssociatedPathOS(link, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	want := "start\n-a\nandroid.intent.action.VIEW\n-d\n" +
		"content://com.android.externalstorage.documents/document/primary:Download%2Fa%20b%231\n" +
		"-t\nvnd.android.document/directory\n"
	if string(got) != want {
		t.Fatalf("am arguments = %q, want %q", got, want)
	}
	if err := openAssociatedPathOS(dir, dir); err == nil {
		t.Fatal("directory outside the shared storage was accepted")
	}
}
