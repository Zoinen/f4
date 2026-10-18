//go:build darwin

package terminal

import (
	"bytes"
	"context"
	"image"
	stdpng "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/unxed/goclip"
)

// The real pasteboard through the real goclip reader. The fixture-driven tests
// cannot see how osascript behaves, and f4#2007 got through them: a clipboard
// with text, copied files or nothing made the reader exit 1 and paste fail.
// The test overwrites the user's clipboard, so it runs only on CI runners.
// Without the fix the text, files and empty cases fail with the osascript exit.
func TestReadClipboardContentsDarwinPasteboard(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("overwrites the system clipboard; runs on CI runners only")
	}
	for _, osa := range []string{"osascript", "pbcopy"} {
		if _, err := exec.LookPath(osa); err != nil {
			t.Skipf("%s is not available: %v", osa, err)
		}
	}
	// internal/testutil leaves a file-backed driver active in every test; the
	// point here is the real macOS one.
	defer goclip.SetActiveDriver(goclip.ActiveDriver())
	goclip.SetActiveDriver(goclip.NewDarwinDriver())
	png := filepath.Join(t.TempDir(), "pixel.png")
	var pixel bytes.Buffer
	if err := stdpng.Encode(&pixel, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(png, pixel.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	set := func(t *testing.T, args ...string) {
		t.Helper()
		if out, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil { // #nosec G204 -- fixed test commands.
			t.Fatalf("%v: %v: %s", args, err, out)
		}
	}
	cases := []struct {
		name      string
		prepare   func(t *testing.T)
		wantImage bool
	}{
		{"text", func(t *testing.T) { set(t, "osascript", "-e", `set the clipboard to "some text"`) }, false},
		{"files", func(t *testing.T) { set(t, "osascript", "-e", `set the clipboard to (POSIX file "`+png+`")`) }, false},
		{"empty", func(t *testing.T) { set(t, "osascript", "-e", `set the clipboard to ""`) }, false},
		{"image", func(t *testing.T) {
			set(t, "osascript", "-e", `set the clipboard to (read (POSIX file "`+png+`") as «class PNGf»)`)
		}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.prepare(t)
			got, err := ReadClipboardContents(context.Background())
			if err != nil {
				t.Fatalf("paste failed: %v", err)
			}
			if (got.Image != nil) != tc.wantImage {
				t.Fatalf("image = %v, want image: %v", got.Image, tc.wantImage)
			}
		})
	}
}
