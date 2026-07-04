package vtui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestQtProtocolRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := map[string]any{
		"type":   "frame",
		"width":  2,
		"height": 1,
		"full":   true,
		"cells":  [][3]uint64{{0, 'A', 0x010203}, {1, 'B', 0x040506}},
	}
	if err := qtSendMessage(&buf, want); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	got, err := qtReadMessage(&buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if qtString(got, "type") != "frame" {
		t.Fatalf("type mismatch: %q", qtString(got, "type"))
	}
	if qtInt(got, "width") != 2 || qtInt(got, "height") != 1 {
		t.Fatalf("size mismatch: %dx%d", qtInt(got, "width"), qtInt(got, "height"))
	}
	if !qtBool(got, "full") {
		t.Fatal("full flag was not preserved")
	}
}

func TestFindQtHostPathEnv(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, qtHostExecutableName())
	if err := os.WriteFile(host, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("F4_QT_HOST_PATH", host)

	got, err := findQtHostPath()
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if got != host {
		t.Fatalf("host path mismatch: got %q want %q", got, host)
	}
}

func TestFindQtHostPathEnvMissing(t *testing.T) {
	t.Setenv("F4_QT_HOST_PATH", filepath.Join(t.TempDir(), "missing-host"))
	if _, err := findQtHostPath(); err == nil {
		t.Fatal("expected missing env path error")
	}
}
