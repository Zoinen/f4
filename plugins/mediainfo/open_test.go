package mediainfo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestResolveMediaPathPreservesQuotedWindowsSeparators(t *testing.T) {
	directory := t.TempDir()
	fs := vfs.NewOSVFS(directory)
	path, err := resolveMediaPath(fs, `"folder\movie file.mp4"`)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(directory, `folder\movie file.mp4`)
	if path != want {
		t.Fatalf("resolved path = %q, want %q", path, want)
	}
}

func TestResolveMediaPathExpandsLocalEnvironment(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("F4_MEDIA_TEST_DIR", directory)
	fs := vfs.NewOSVFS(t.TempDir())
	for _, raw := range []string{`${F4_MEDIA_TEST_DIR}/movie.mp4`, `%F4_MEDIA_TEST_DIR%/movie.mp4`} {
		path, err := resolveMediaPath(fs, raw)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(directory, "movie.mp4")
		if path != want {
			t.Fatalf("resolveMediaPath(%q) = %q, want %q", raw, path, want)
		}
	}
}

func TestAvailableReportPathAvoidsExistingFile(t *testing.T) {
	directory := t.TempDir()
	fs := vfs.NewOSVFS(directory)
	source := filepath.Join(directory, "movie.mp4")
	if err := os.WriteFile(filepath.Join(directory, "movie.MediaInfo.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := availableReportPath(context.Background(), fs, source)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(directory, "movie.MediaInfo (2).txt")
	if path != want {
		t.Fatalf("report path = %q, want %q", path, want)
	}
}

func TestExpandOSPathEnvironmentDoesNotLoopOnSelfReference(t *testing.T) {
	t.Setenv("F4_MEDIA_LOOP", "%F4_MEDIA_LOOP%")
	if got := expandOSPathEnvironment("%F4_MEDIA_LOOP%"); got != "%F4_MEDIA_LOOP%" {
		t.Fatalf("self reference = %q", got)
	}
}
