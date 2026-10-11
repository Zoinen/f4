package ap

// Line-ending behaviour of modifications that leave the text unchanged
// (f4#1606, found by the Windows probe on 56_insert_noop). The fixtures are
// written byte for byte into t.TempDir() rather than taken from testdata/,
// so a checkout's core.autocrlf cannot change what these tests exercise.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// applyToBytes writes src as target.txt, applies patchBody (everything after
// the "AP 3.2" header, the FILE line and the path) in tolerant mode and
// returns the resulting file bytes.
func applyToBytes(t *testing.T, fileArg, src, patchBody string) string {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte(src), 0o600); err != nil {
		t.Fatalf("setup target: %v", err)
	}
	patch := "c0de0001 AP 3.2\n\nc0de0001 FILE" + fileArg + "\ntarget.txt\n\n" + patchBody
	patchPath := filepath.Join(dir, "_case.ap")
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}
	var buf bytes.Buffer
	result := Apply(patchPath, dir, Options{Out: &buf})
	if result.Status != StatusSuccess {
		t.Fatalf("status = %s, want %s (out: %s)", result.Status, StatusSuccess, buf.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	return string(got)
}

// TestNoOpKeepsFileBytes: a patch whose modifications all leave the text as
// it was must not touch the file, whatever its line endings. It used to
// hold only for LF files: the "did anything change" check compared the
// re-joined CRLF/CR text against the LF-normalized original, so a CRLF or CR
// file always counted as changed and got the trailing newline the patcher
// only adds to files it really rewrites.
func TestNoOpKeepsFileBytes(t *testing.T) {
	insertAfter := "c0de0001 INSERT_AFTER\nc0de0001 snippet\nLine 1\nc0de0001 content\nc0de0001 END\n"
	insertBefore := "c0de0001 INSERT_BEFORE\nc0de0001 snippet\nLine 2\nc0de0001 content\nc0de0001 END\n"
	recreateSame := "c0de0001 RECREATE\nc0de0001 content\nLine 1\nLine 2\nc0de0001 END\n"
	cases := []struct {
		name, src, patch string
	}{
		{"insert_after_empty_lf_no_final_newline", "Line 1\nLine 2", insertAfter},
		{"insert_after_empty_crlf_no_final_newline", "Line 1\r\nLine 2", insertAfter},
		{"insert_after_empty_crlf", "Line 1\r\nLine 2\r\n", insertAfter},
		{"insert_before_empty_crlf_no_final_newline", "Line 1\r\nLine 2", insertBefore},
		{"insert_after_empty_cr_no_final_newline", "Line 1\rLine 2", insertAfter},
		{"insert_after_empty_mixed_endings", "Line 1\r\nLine 2\nLine 3", insertAfter},
		{"recreate_same_content_crlf_no_final_newline", "Line 1\r\nLine 2", recreateSame},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := applyToBytes(t, "", c.src, c.patch); got != c.src {
				t.Fatalf("file changed by a no-op patch:\n got %q\nwant %q", got, c.src)
			}
		})
	}
}

// TestExplicitNewlineStillConverts: an explicit FILE LF/CRLF is a request
// to change the line endings, so it rewrites the file even when no
// modification changes the text (the no-op check above must not swallow it).
func TestExplicitNewlineStillConverts(t *testing.T) {
	insertAfter := "c0de0001 INSERT_AFTER\nc0de0001 snippet\nLine 1\nc0de0001 content\nc0de0001 END\n"
	if got, want := applyToBytes(t, " LF", "Line 1\r\nLine 2\r\n", insertAfter), "Line 1\nLine 2\n"; got != want {
		t.Fatalf("FILE LF on a CRLF file: got %q, want %q", got, want)
	}
	if got, want := applyToBytes(t, " CRLF", "Line 1\nLine 2", insertAfter), "Line 1\r\nLine 2\r\n"; got != want {
		t.Fatalf("FILE CRLF on an LF file: got %q, want %q", got, want)
	}
}

// TestCRLFMatchesLF: the neighbouring modes that do change the text
// (REPLACE with empty content, which tolerant mode turns into DELETE, and
// DELETE itself, on the last line with and without a final newline) give on
// a CRLF file exactly the LF result with CRLF line endings.
func TestCRLFMatchesLF(t *testing.T) {
	deleteLast := "c0de0001 DELETE\nc0de0001 snippet\nLine 3\n"
	replaceLastEmpty := "c0de0001 REPLACE\nc0de0001 snippet\nLine 3\nc0de0001 content\nc0de0001 END\n"
	deleteMiddle := "c0de0001 DELETE\nc0de0001 snippet\nLine 2\n"
	insertLast := "c0de0001 INSERT_AFTER\nc0de0001 snippet\nLine 3\nc0de0001 content\nLine 4\n"
	cases := []struct {
		name, lfSrc, patch string
	}{
		{"delete_last_line", "Line 1\nLine 2\nLine 3\n", deleteLast},
		{"delete_last_line_no_final_newline", "Line 1\nLine 2\nLine 3", deleteLast},
		{"replace_last_line_empty", "Line 1\nLine 2\nLine 3\n", replaceLastEmpty},
		{"replace_last_line_empty_no_final_newline", "Line 1\nLine 2\nLine 3", replaceLastEmpty},
		{"delete_middle_line_no_final_newline", "Line 1\nLine 2\nLine 3", deleteMiddle},
		{"insert_after_last_line_no_final_newline", "Line 1\nLine 2\nLine 3", insertLast},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lf := applyToBytes(t, "", c.lfSrc, c.patch)
			if lf == c.lfSrc {
				t.Fatalf("LF run left the file unchanged (%q); the case does not exercise a change", lf)
			}
			crlfSrc := strings.ReplaceAll(c.lfSrc, "\n", "\r\n")
			want := strings.ReplaceAll(lf, "\n", "\r\n")
			if got := applyToBytes(t, "", crlfSrc, c.patch); got != want {
				t.Fatalf("CRLF result differs from the LF one:\n got %q\nwant %q (LF: %q)", got, want, lf)
			}
		})
	}
}
