package main

import (
	"strings"
	"testing"
)

func TestFormatGroupsNamespacesAndKeepsValues(t *testing.T) {
	source := "[Language]\nName=English\nCode=en\nAuthor=f4 Team\n\n[Strings]\nVisRen.Menu=menu\nID3Editor.Title=title\nVisRen.Title=title\nMenu.Files=files\n"
	translation := "[Language]\nName=Test\nCode=xx\nAuthor=Test\n\n[Strings]\nVisRen.Title=translated title\nMenu.Files=translated files\nVisRen.Menu=translated menu\n"

	sourceFile, err := parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	f, err := newFormatter(sourceFile.entries)
	if err != nil {
		t.Fatal(err)
	}
	translationFile, err := parse([]byte(translation))
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := f.format(translationFile)
	if err != nil {
		t.Fatal(err)
	}

	want := "[Language]\nName=Test\nCode=xx\nAuthor=Test\n\n[Strings]\nVisRen.Menu=translated menu\nVisRen.Title=translated title\n\nMenu.Files=translated files\n"
	if string(formatted) != want {
		t.Fatalf("formatted language differs\n got: %q\nwant: %q", formatted, want)
	}
}

func TestFormatPreservesCRLFAndComments(t *testing.T) {
	source := "[Strings]\r\nA.One=one\r\nB.Two=two\r\nA.Three=three\r\n"
	translation := "[Strings]\r\n; keep with A\r\nA.Three=3\r\nB.Two=2\r\n"

	sourceFile, err := parse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	f, err := newFormatter(sourceFile.entries)
	if err != nil {
		t.Fatal(err)
	}
	translationFile, err := parse([]byte(translation))
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := f.format(translationFile)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(formatted), "\r\n") {
		t.Fatalf("formatter did not preserve CRLF line endings: %q", formatted)
	}
	want := "[Strings]\r\n; keep with A\r\nA.Three=3\r\n\r\nB.Two=2\r\n"
	if string(formatted) != want {
		t.Fatalf("formatted CRLF language differs\n got: %q\nwant: %q", formatted, want)
	}
}

func TestFormatRejectsUnknownKey(t *testing.T) {
	sourceFile, err := parse([]byte("[Strings]\nKnown=value\n"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := newFormatter(sourceFile.entries)
	if err != nil {
		t.Fatal(err)
	}
	translationFile, err := parse([]byte("[Strings]\nUnknown=value\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.format(translationFile); err == nil || !strings.Contains(err.Error(), "missing from English source") {
		t.Fatalf("format error = %v, want unknown-key error", err)
	}
}

func TestFormatterRejectsDuplicateSourceKey(t *testing.T) {
	entries := []entry{{key: "Known", line: "Known=value"}, {key: "Known", line: "Known=again"}}
	if _, err := newFormatter(entries); err == nil || !strings.Contains(err.Error(), "duplicate source key") {
		t.Fatalf("formatter error = %v, want duplicate-source-key error", err)
	}
}
