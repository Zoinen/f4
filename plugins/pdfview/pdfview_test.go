package pdfview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/pdftext"
	"github.com/unxed/f4/vfs"
)

func stream(dict, body string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
}

// samplePDF is a one-page file with a line of text and one JPEG picture.
func samplePDF() string {
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> /XObject << /Im0 6 0 R >> >> >>",
		stream("", "BT /F1 12 Tf (Hello PDF) Tj ET"),
		"<< /Type /Font /Subtype /Type1 >>",
		stream("/Type /XObject /Subtype /Image /Width 8 /Height 8 /Filter /DCTDecode", "JPEGBYTES"),
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	for i, o := range objs {
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	b.WriteString("trailer\n<< /Root 1 0 R >>\n")
	return b.String()
}

func listing(t *testing.T, v vfs.VFS, dir string) map[string]bool {
	t.Helper()
	got := map[string]bool{}
	if err := v.ReadDir(context.Background(), dir, func(items []vfs.VFSItem) {
		for _, it := range items {
			got[it.Name] = it.IsDir
		}
	}); err != nil {
		t.Fatalf("ReadDir(%q): %v", dir, err)
	}
	return got
}

func readAll(t *testing.T, v vfs.VFS, p string) string {
	t.Helper()
	ctx := context.Background()
	f, err := v.Open(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var sb strings.Builder
	buf := make([]byte, 7)
	for {
		n, err := f.Read(ctx, buf)
		sb.Write(buf[:n])
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return sb.String()
}

func TestProviderMountsAPDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(path, []byte(samplePDF()), 0o600); err != nil {
		t.Fatal(err)
	}
	local := vfs.NewOSVFS(dir)
	p := &provider{}
	ctx := context.Background()
	if !p.CanOpen(ctx, local, path) || p.PanelEnterAllowed(ctx, local, path) || p.Name() != "pdfview" || p.Priority() == 0 {
		t.Fatal("provider does not claim a PDF on Ctrl+PgDn only")
	}
	v, err := p.Open(ctx, local, path)
	if err != nil {
		t.Fatal(err)
	}
	root := listing(t, v, "/")
	if root["text.md"] || !root["Pages"] || !root["Images"] {
		t.Fatalf("root = %v", root)
	}
	if !strings.Contains(readAll(t, v, "/Pages/page-001.txt"), "Hello PDF") {
		t.Error("page text missing")
	}
	if !strings.Contains(readAll(t, v, "/text.md"), "Hello PDF") {
		t.Error("report text missing")
	}
	if got := readAll(t, v, "/Images/p001-Im0.jpg"); got != "JPEGBYTES" {
		t.Errorf("picture = %q", got)
	}
	titled, ok := v.(interface {
		PanelTitle(string) string
		GetTitle() string
	})
	if !ok || titled.PanelTitle("/Pages") != "PDF:doc.pdf/Pages" || titled.PanelTitle("/") != "PDF:doc.pdf" || titled.GetTitle() != "doc.pdf" {
		t.Error("titles wrong")
	}
}

func TestTreeBrowsingAndReadOnly(t *testing.T) {
	v := newDocumentVFS(nil, "d.pdf", &pdftext.Result{Pages: []string{"one", ""}}, nil)
	if root := listing(t, v, "/"); root["Images"] != false || len(root) != 2 {
		t.Errorf("a PDF without pictures lists %v", root)
	}
	if !v.IsAtRoot() || v.GetPath() != "/" {
		t.Error("not at the root")
	}
	if err := v.SetPath("/Pages"); err != nil || v.IsAtRoot() {
		t.Errorf("SetPath: %v", err)
	}
	if err := v.SetPath("/text.md"); err == nil {
		t.Error("SetPath into a file succeeded")
	}
	if err := v.SetPath("/nowhere"); !errors.Is(err, os.ErrInvalid) {
		t.Errorf("SetPath into nothing = %v", err)
	}
	ctx := context.Background()
	if it, err := v.Stat(ctx, "/Pages/page-001.txt"); err != nil || it.IsDir || it.Size != 4 {
		t.Errorf("Stat = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "/"); err != nil || !it.IsDir || it.Name != "d.pdf" {
		t.Errorf("root Stat = %+v, %v", it, err)
	}
	if _, err := v.Stat(ctx, "/x"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat missing = %v", err)
	}
	if _, err := v.Open(ctx, "/Pages"); err == nil {
		t.Error("a folder opened as a file")
	}
	f, err := v.Open(ctx, "/Pages/page-001.txt")
	if err != nil {
		t.Fatal(err)
	}
	if n, err := f.ReadAt(ctx, make([]byte, 2), 99); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt past the end = %d, %v", n, err)
	}
	_ = f.Close()
	for name, err := range map[string]error{
		"MkDir":         v.MkDir(ctx, "/x"),
		"Remove":        v.Remove(ctx, "/x"),
		"Rename":        v.Rename(ctx, "/x", "/y"),
		"SetAttributes": v.SetAttributes(ctx, "/x", vfs.VFSItem{}),
	} {
		if err == nil {
			t.Errorf("%s succeeded on a read-only panel", name)
		}
	}
	if _, err := v.Create(ctx, "/n"); err == nil {
		t.Error("Create succeeded")
	}
	if !v.GetCapabilities().HasRandomAccess || v.ParentVFS() != nil || v.Close() != nil {
		t.Error("capabilities, parent or Close wrong")
	}
	if ch, err := v.Search(ctx, "/", "x"); ch != nil || err != nil {
		t.Error("Search is not a no-op")
	}
	if clone := v.Clone(); clone.GetPath() != v.GetPath() {
		t.Error("clone lost the path")
	}
	root := newDir()
	root.add("a/b", file("1"))
	root.add("a/b", file("2"))
	root.add("", file("3"))
	if len(root.names) != 3 || root.names[0] != "a_b" || root.names[1] != "a_b (2)" {
		t.Errorf("names = %v", root.names)
	}
}

func TestProviderRefusals(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake.pdf")
	other := filepath.Join(dir, "doc.txt")
	for _, p := range []string{fake, other} {
		if err := os.WriteFile(p, []byte("plain text"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	local := vfs.NewOSVFS(dir)
	p := &provider{}
	ctx := context.Background()
	if p.CanOpen(ctx, local, fake) || p.CanOpen(ctx, local, other) || p.CanOpen(ctx, nil, fake) || p.CanOpen(ctx, local, filepath.Join(dir, "no.pdf")) {
		t.Error("a file that is not a PDF was claimed")
	}
	if _, err := p.Open(ctx, local, other); !errors.Is(err, pdftext.ErrNotPDF) {
		t.Errorf("Open(text) = %v", err)
	}
	if _, err := p.Open(ctx, local, fake); !errors.Is(err, pdftext.ErrNotPDF) {
		t.Errorf("Open(fake) = %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if p.CanOpen(cancelled, local, fake) {
		t.Error("a cancelled context still claimed a file")
	}
	pl := NewPlugin()
	if pl.GetName() == "" || pl.Init(nil) == nil || pl.Close() != nil {
		t.Error("plugin lifecycle wrong")
	}
}
