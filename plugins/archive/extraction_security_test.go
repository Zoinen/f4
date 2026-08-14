package archive

import (
	stdtar "archive/tar"
	stdzip "archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestArchiveVFSCopyBulkRejectsTraversal(t *testing.T) {
	tests := []struct {
		name        string
		archiveName string
		write       func(*testing.T, string)
		wantFormat  string
	}{
		{name: "zip", archiveName: "malicious.zip", write: writeTraversalZIP, wantFormat: "zip"},
		{name: "tar", archiveName: "malicious.tar", write: writeTraversalTAR, wantFormat: "tar"},
		// An extensionless ZIP is deliberately dispatched through the generic
		// archives.Extractor branch: the constructor detects its backing by
		// content, while CopyBulk has no display-name format hint.
		{name: "fallback", archiveName: "malicious.payload", write: writeTraversalZIP, wantFormat: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := t.TempDir()
			archivePath := filepath.Join(workspace, test.archiveName)
			test.write(t, archivePath)

			destination := filepath.Join(workspace, "extracted")
			if err := os.MkdirAll(destination, 0o755); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(workspace, "escaped.txt")
			const sentinel = "must not be overwritten"
			if err := os.WriteFile(outside, []byte(sentinel), 0o600); err != nil {
				t.Fatal(err)
			}

			archiveVFS, err := NewArchiveVFS(vfs.NewOSVFS(workspace), test.archiveName)
			if err != nil {
				t.Fatalf("open malicious fixture: %v", err)
			}
			defer archiveVFS.Close()
			if archiveVFS.format != test.wantFormat {
				t.Fatalf("fixture dispatch format = %q, want %q", archiveVFS.format, test.wantFormat)
			}

			err = archiveVFS.CopyBulk(
				context.Background(),
				[]string{"safe"},
				vfs.NewOSVFS(destination),
				destination,
				&dummyReporter{},
			)
			if err == nil {
				t.Fatal("CopyBulk accepted an archive member which escapes the destination")
			}

			contents, readErr := os.ReadFile(outside)
			if readErr != nil {
				t.Fatalf("read outside sentinel: %v", readErr)
			}
			if string(contents) != sentinel {
				t.Fatalf("archive traversal overwrote outside file: %q", contents)
			}
		})
	}
}

func TestCleanArchiveExtractionPathRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{
		"../outside.txt",
		"safe/../../outside.txt",
		`safe\..\..\outside.txt`,
		"/absolute.txt",
		`C:\absolute.txt`,
		`\\server\share\absolute.txt`,
		"nul\x00name",
	} {
		t.Run(name, func(t *testing.T) {
			if cleaned, err := cleanArchiveExtractionPath(name); err == nil {
				t.Fatalf("unsafe path normalized to %q", cleaned)
			}
		})
	}

	cleaned, err := cleanArchiveExtractionPath("safe/../inside.txt")
	if err != nil {
		t.Fatalf("safe normalized path rejected: %v", err)
	}
	if cleaned != "inside.txt" {
		t.Fatalf("safe normalized path = %q, want inside.txt", cleaned)
	}
}

func writeTraversalZIP(t *testing.T, archivePath string) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := stdzip.NewWriter(file)
	writeZIPMember(t, writer, "safe/ok.txt", "safe")
	writeZIPMember(t, writer, "safe/../../escaped.txt", "owned")
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeZIPMember(t *testing.T, writer *stdzip.Writer, name, contents string) {
	t.Helper()
	member, err := writer.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(member, contents); err != nil {
		t.Fatal(err)
	}
}

func writeTraversalTAR(t *testing.T, archivePath string) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := stdtar.NewWriter(file)
	writeTARMember(t, writer, "safe/ok.txt", "safe")
	writeTARMember(t, writer, "safe/../../escaped.txt", "owned")
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTARMember(t *testing.T, writer *stdtar.Writer, name, contents string) {
	t.Helper()
	header := &stdtar.Header{Name: name, Mode: 0o644, Size: int64(len(contents))}
	if err := writer.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, contents); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}
