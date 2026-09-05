package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestWriteFileAtomicallyPublishesCompleteData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.ini")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want := []byte(strings.Repeat("new\n", 1000))
	if err := writeFileAtomically(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("published data was incomplete: got %d bytes, want %d", len(got), len(want))
	}
	mode, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && mode.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", mode.Mode().Perm())
	}
	if leftovers, err := filepath.Glob(filepath.Join(dir, ".f4-atomic-*")); err != nil {
		t.Fatal(err)
	} else if len(leftovers) != 0 {
		t.Fatalf("atomic temporary files remain: %v", leftovers)
	}
}

func TestWriteFileAtomicallyConcurrentWritersNeverPublishPartialData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.ini")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			letters := "ABCDEFGHIJKLMNOP"
			data := []byte(strings.Repeat(string(letters[i]), 4096))
			if err := writeFileAtomically(path, data, 0o600); err != nil {
				t.Errorf("writer %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4096 {
		t.Fatalf("final file length = %d, want 4096", len(got))
	}
	for _, b := range got {
		if b != got[0] {
			t.Fatalf("final file contains a partial mix of writers")
		}
	}
}
