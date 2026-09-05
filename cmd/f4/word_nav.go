package main

import "strings"

// Character classes used for word navigation. far2l never puts whitespace
// into the word divider set, so spaces form a class of their own.
const (
	catSpace = iota
	catDivider
	catWord
)

// DefaultWordDiv repeats the default value of Opt.strWordDiv in far2l.
const DefaultWordDiv = "~!%^&*()+|{}:\"<>?`-=\\[];',./"

func getCharCategory(r rune) int {
	if r == ' ' || r == '\t' {
		return catSpace
	}
	if strings.ContainsRune(DefaultWordDiv, r) {
		return catDivider
	}
	return catWord
}

// stopBeforeRuneLeft reports whether a leftward word jump must stop with the
// cursor on curr, prev being the rune right before it.
//
// When selecting is false this repeats the loop of Edit::ProcessKey for
// KEY_CTRLLEFT in far2l edit.cpp. When selecting is true it repeats the
// SkipSpace loop of Editor::ProcessKey for KEY_CTRLSHIFTLEFT in editor.cpp,
// which treats dividers exactly like spaces so that a selection always covers
// whole words.
func stopBeforeRuneLeft(prev, curr rune, selecting bool) bool {
	pCat, cCat := getCharCategory(prev), getCharCategory(curr)
	if selecting {
		return pCat != catWord && cCat == catWord
	}
	return (pCat == catSpace && cCat != catSpace) ||
		(pCat == catDivider && cCat == catWord)
}

// stopBeforeRuneRight is the rightward counterpart of stopBeforeRuneLeft,
// mirroring KEY_CTRLRIGHT and KEY_CTRLSHIFTRIGHT in far2l.
func stopBeforeRuneRight(prev, curr rune, selecting bool) bool {
	pCat, cCat := getCharCategory(prev), getCharCategory(curr)
	if selecting {
		return pCat == catWord && cCat != catWord
	}
	return (pCat == catSpace && cCat != catSpace) ||
		(pCat == catWord && cCat == catDivider)
}
