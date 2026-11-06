package filegroups

import (
	"bytes"
	"strings"
	"testing"
)

func TestDocumentHelpPreservesPersonalCommentsAndBOM(t *testing.T) {
	const personal = "# Personal header\n; personal settings\n"
	const body = "\n[FileGroups]\nVersion=2\n# section comment\n[Unknown]\nKeep=C:\\Tools\\file\n"
	oldHelp := documentHelpStart + "\n# old generated documentation\n" + documentHelpEnd + "\n"
	for _, tc := range []struct {
		name, source string
	}{
		{name: "first save", source: personal + body},
		{name: "refresh", source: personal + oldHelp + body},
		{name: "BOM", source: "\ufeff" + personal + oldHelp + body},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := refreshDocumentHelp([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(got, []byte(personal)) || !bytes.HasSuffix(got, []byte(body)) {
				t.Fatal("personal comments or document body changed")
			}
			if strings.HasPrefix(tc.source, "\ufeff") != bytes.HasPrefix(got, []byte("\ufeff")) {
				t.Fatal("BOM changed")
			}
			if bytes.Contains(got, []byte("old generated documentation")) {
				t.Fatal("stale help retained inside the managed block")
			}
			if _, err := Parse(got); err != nil {
				t.Fatalf("refreshed help broke INI syntax: %v", err)
			}
			again, err := refreshDocumentHelp(got)
			if err != nil || !bytes.Equal(again, got) {
				t.Fatalf("help refresh is not idempotent: %v", err)
			}
		})
	}
}

func TestDocumentHelpRefusesMalformedBlocks(t *testing.T) {
	for _, tc := range []struct {
		name, source string
	}{
		{name: "missing end", source: documentHelpStart + "\n# custom\n[FileGroups]\nVersion=2\n"},
		{name: "missing start", source: documentHelpEnd + "\n[FileGroups]\nVersion=2\n"},
		{name: "duplicate start", source: documentHelpStart + "\n" + documentHelpStart + "\n" + documentHelpEnd + "\n"},
		{name: "duplicate end", source: documentHelpStart + "\n" + documentHelpEnd + "\n" + documentHelpEnd + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if data, err := refreshDocumentHelp([]byte(tc.source)); err == nil || data != nil {
				t.Fatal("malformed markers could erase personal comments")
			}
		})
	}
}

func TestDocumentHelpNeverRewritesSectionComments(t *testing.T) {
	source := "[Unknown]\n" + documentHelpStart + "\n# personal section comment\n" + documentHelpEnd + "\n"
	got, err := refreshDocumentHelp([]byte(source))
	if err != nil || !bytes.HasSuffix(got, []byte(source)) {
		t.Fatalf("comments inside a section were treated as generated preamble: %v", err)
	}
}
