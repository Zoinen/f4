package panel

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func base64TestPanel(root, name string) *PanelsFrame {
	fp := &FileSystemPanel{
		Vfs:           vfs.NewOSVFS(root),
		Entries:       []*FileEntry{{VFSItem: vfs.VFSItem{Name: name}}},
		ViewMode:      ViewModeDetailed,
		SelectedItems: make(map[string]bool),
		Table:         vtui.NewTable(1, 1, 38, 8, nil),
	}
	fp.ScreenObject.SetPosition(0, 0, 39, 11)
	fp.initScrollBar()
	return &PanelsFrame{Panels: [2]Panel{fp, nil}, ActiveIdx: 0}
}

type base64ReadAtCloser struct {
	data   []byte
	closed bool
}

func (r *base64ReadAtCloser) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *base64ReadAtCloser) Read(_ context.Context, p []byte) (int, error) {
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *base64ReadAtCloser) Close() error {
	r.closed = true
	return nil
}

func (r *base64ReadAtCloser) Size() int64 { return int64(len(r.data)) }

func TestBase64FileDestinationForEncoding(t *testing.T) {
	fs := vfs.NewOSVFS(t.TempDir())
	if got := base64FileDestination(fs, fs.Join(fs.GetPath(), "file.txt"), true); !strings.HasSuffix(got, "file.txt.b64") {
		t.Fatalf("encoded destination = %q", got)
	}
}

func TestBase64FileDestinationRemovesSuffixCaseInsensitively(t *testing.T) {
	fs := vfs.NewOSVFS(t.TempDir())
	got := base64FileDestination(fs, fs.Join(fs.GetPath(), "FILE.B64"), false)
	if !strings.HasSuffix(got, "FILE") {
		t.Fatalf("decoded destination = %q", got)
	}
}

func TestBase64FileDestinationAddsDecodedSuffix(t *testing.T) {
	fs := vfs.NewOSVFS(t.TempDir())
	got := base64FileDestination(fs, fs.Join(fs.GetPath(), "file.txt"), false)
	if !strings.HasSuffix(got, "file.txt.decoded") {
		t.Fatalf("non-b64 destination = %q", got)
	}
}

func TestTransformBase64FileDataAcceptsUnpaddedRawEncoding(t *testing.T) {
	original := []byte("raw base64")
	padded := base64.StdEncoding.EncodeToString(original)
	unpadded := strings.TrimRight(padded, "=")
	got, err := transformBase64FileData([]byte(unpadded), false)
	if err != nil {
		t.Fatalf("raw decode: %v", err)
	}
	if string(got) != string(original) {
		t.Fatalf("decoded = %q, want %q", got, original)
	}
}

func TestBase64ContextReaderForwardsReadsAndClose(t *testing.T) {
	underlying := &base64ReadAtCloser{data: []byte("payload")}
	reader := contextReader{ctx: context.Background(), reader: underlying}
	got, err := io.ReadAll(reader)
	if err != nil || string(got) != "payload" {
		t.Fatalf("ReadAll = %q, %v", got, err)
	}
	if err := underlying.Close(); err != nil || !underlying.closed {
		t.Fatalf("Close = %v, closed=%v", err, underlying.closed)
	}
}

func TestTransformSelectedFileRejectsNilFrame(t *testing.T) {
	if _, err := TransformSelectedFileBase64(nil, true); err != errBase64FileNoSelection {
		t.Fatalf("nil frame error = %v", err)
	}
}

func TestTransformSelectedFileRejectsMissingActiveVFS(t *testing.T) {
	pf := &PanelsFrame{}
	if _, err := TransformSelectedFileBase64(pf, true); err != errBase64FileNoSelection {
		t.Fatalf("missing active panel error = %v", err)
	}
	pf.Panels[0] = &FileSystemPanel{}
	if _, err := TransformSelectedFileBase64(pf, true); err != errBase64FileNoSelection {
		t.Fatalf("missing VFS error = %v", err)
	}
}

func TestTransformSelectedFileRejectsMissingSelection(t *testing.T) {
	root := t.TempDir()
	pf := base64TestPanel(root, "")
	if _, err := TransformSelectedFileBase64(pf, true); err != errBase64FileNoSelection {
		t.Fatalf("empty selection error = %v", err)
	}
	pf.GetActivePanel().Entries[0].Name = ".."
	if _, err := TransformSelectedFileBase64(pf, true); err != errBase64FileNoSelection {
		t.Fatalf("parent selection error = %v", err)
	}
}

func TestTransformSelectedFileRejectsStatErrorsAndDirectories(t *testing.T) {
	root := t.TempDir()
	pf := base64TestPanel(root, "missing.txt")
	if _, err := TransformSelectedFileBase64(pf, true); err == nil || !strings.Contains(err.Error(), "stat source file") {
		t.Fatalf("missing source error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	pf.GetActivePanel().Entries[0].Name = "folder"
	if _, err := TransformSelectedFileBase64(pf, true); err != errBase64FileDirectory {
		t.Fatalf("directory error = %v", err)
	}
}

func TestTransformSelectedFileRoundTripsSelectedFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample.txt"), []byte("hello base64"), 0o600); err != nil {
		t.Fatal(err)
	}
	pf := base64TestPanel(root, "sample.txt")
	encoded, err := TransformSelectedFileBase64(pf, true)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	encodedData, err := os.ReadFile(encoded)
	if err != nil || string(encodedData) != base64.StdEncoding.EncodeToString([]byte("hello base64")) {
		t.Fatalf("encoded file = %q, %v", encodedData, err)
	}
	if err := os.Remove(filepath.Join(root, "sample.txt")); err != nil {
		t.Fatal(err)
	}
	pf.GetActivePanel().Entries[0].Name = filepath.Base(encoded)
	decoded, err := TransformSelectedFileBase64(pf, false)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	decodedData, err := os.ReadFile(decoded)
	if err != nil || string(decodedData) != "hello base64" {
		t.Fatalf("decoded file = %q, %v", decodedData, err)
	}
}
