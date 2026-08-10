package vtui

import "strings"

// Character classes used for word navigation. far2l never puts whitespace into
// the word divider set, so spaces form a class of their own.
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
// cursor on curr, prev being the rune right before it. It repeats the loops of
// Edit::ProcessKey in far2l edit.cpp: KEY_CTRLLEFT when selecting is false,
// KEY_CTRLSHIFTLEFT when it is true.
//
// The selecting variant is deliberately finer than plain movement: its inner
// break also fires on a divider, so every boundary inside a run of dividers is
// a stop. The multi-line editor uses the opposite rule and lives elsewhere.
func stopBeforeRuneLeft(prev, curr rune, selecting bool) bool {
	pCat, cCat := getCharCategory(prev), getCharCategory(curr)
	if pCat == catSpace && cCat != catSpace {
		return true
	}
	if pCat == catDivider && cCat == catWord {
		return true
	}
	return selecting && pCat == catDivider && cCat == catDivider
}

// stopBeforeRuneRight is the rightward counterpart of stopBeforeRuneLeft,
// mirroring KEY_CTRLRIGHT and KEY_CTRLSHIFTRIGHT in far2l edit.cpp.
func stopBeforeRuneRight(prev, curr rune, selecting bool) bool {
	pCat, cCat := getCharCategory(prev), getCharCategory(curr)
	if pCat == catSpace && cCat != catSpace {
		return true
	}
	if pCat == catWord && cCat == catDivider {
		return true
	}
	return selecting && pCat == catDivider && cCat != catSpace
}
