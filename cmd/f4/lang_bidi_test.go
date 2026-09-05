package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// bidiControls are the invisible characters that steer bidirectional text.
// They must not appear in the translation data, for three reasons:
//
//   - they are invisible, so nobody reviewing a .lng file can see them, and
//     they survive copy-paste from a browser into a translation without any
//     trace in the diff except a suspicious byte count;
//   - a TUI budgets screen columns per rune. A zero-width control that the
//     renderer does not know about either eats a column it should not or
//     prints as a replacement box, and the dialog layout tests cannot see
//     either case because they measure the string, not the screen;
//   - reordering is the renderer's job. If a string ever genuinely needs an
//     isolate around an LTR run inside RTL text, vtui has to insert it while
//     drawing, uniformly for every language, instead of one translator
//     guessing for one string.
//
// The first offender was ar.lng, where Bookmarks.RowPrefix carried an LRM on
// each side of an all-ASCII key list. No other language did anything of the
// kind for the same string.
//
// help/*.hlf is scanned too, since the homoglyph backlog those files carried
// has been worked off and lang/homoglyph_baseline.txt is gone. A help topic
// is the likelier place for a stray control to survive: it is prose rather
// than a caption, it is long, and much of it was pasted in from a browser.
var bidiControls = map[rune]string{
	'\u061C': "ALM",
	'\u200E': "LRM",
	'\u200F': "RLM",
	'\u202A': "LRE",
	'\u202B': "RLE",
	'\u202C': "PDF",
	'\u202D': "LRO",
	'\u202E': "RLO",
	'\u2066': "LRI",
	'\u2067': "RLI",
	'\u2068': "FSI",
	'\u2069': "PDI",
}

func TestTranslationsHaveNoBidiControls(t *testing.T) {
	skipIfNoRelevantChanges(t, "lang_bidi",
		"lang/*.lng",
		"help/*.hlf",
		"lang_bidi_test.go",
	)

	paths, err := filepath.Glob("lang/*.lng")
	if err != nil {
		t.Fatalf("cannot list language files: %v", err)
	}
	helpPaths, err := filepath.Glob("help/*.hlf")
	if err != nil {
		t.Fatalf("cannot list help files: %v", err)
	}
	paths = append(paths, helpPaths...)
	if len(paths) == 0 {
		t.Fatal("no localization files found, is the test running from the repository root?")
	}
	sort.Strings(paths)

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("cannot read %s: %v", path, err)
			continue
		}

		for i, line := range strings.Split(string(data), "\n") {
			for _, r := range line {
				name, isControl := bidiControls[r]
				if !isControl {
					continue
				}
				where := strings.TrimSpace(line)
				if idx := strings.Index(line, "="); idx > 0 && strings.HasSuffix(path, ".lng") {
					where = line[:idx]
				}
				t.Errorf("%s:%d: %q carries a %s (%s), an invisible bidi control; "+
					"strip it and let the renderer order the text",
					path, i+1, where, name, fmt.Sprintf("U+%04X", r))
			}
		}
	}
}
