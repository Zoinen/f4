package panel

import (
	"testing"

	"github.com/unxed/f4/internal/config"
)

// f4 #1405: PanelStrictAutoFilter opts fastFindMatch out of the fuzzy
// matcher's typo tolerance. "tes" and "des" are one substitution apart, so
// the default (fuzzy) matcher accepts "destroy" for query "tes"; strict mode
// must reject it while still accepting the exact substring in "test".
func TestFastFindMatch_StrictRejectsOneEditTypo(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()

	fp := &FileSystemPanel{}
	fp.FastFindMode = true
	fp.FastFindStr = "*tes"

	config.App.PanelStrictAutoFilter = false
	if _, _, ok := fp.fastFindMatch("test"); !ok {
		t.Error("fuzzy matcher rejected the exact substring \"tes\" in \"test\"")
	}
	if _, _, ok := fp.fastFindMatch("destroy"); !ok {
		t.Error("fuzzy (default) matcher rejected \"tes\" against \"destroy\" (1 edit) — is the tolerance gone?")
	}

	// Changing the setting must invalidate the cached matchers built above.
	config.App.PanelStrictAutoFilter = true
	if _, _, ok := fp.fastFindMatch("test"); !ok {
		t.Error("strict matcher rejected the exact substring \"tes\" in \"test\"")
	}
	if _, _, ok := fp.fastFindMatch("destroy"); ok {
		t.Error("strict matcher accepted \"tes\" against \"destroy\" (1 edit away) — typo tolerance leaked through")
	}
}

// f4 #1709: the keyboard-layout transcoding ("сфы" typed on a Russian layout
// also searches "cas") is a tolerance, so strict mode must not apply it: it
// searches exactly what was typed.
func TestFastFindMatch_StrictDoesNotTranscodeLayout(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()

	fp := &FileSystemPanel{}
	fp.FastFindMode = true
	fp.FastFindStr = "*сфы"

	config.App.PanelStrictAutoFilter = false
	if _, _, ok := fp.fastFindMatch("Cas"); !ok {
		t.Skip("layout transcoding is unavailable in this environment")
	}

	config.App.PanelStrictAutoFilter = true
	if _, _, ok := fp.fastFindMatch("Cas"); ok {
		t.Error("strict matcher found \"Cas\" for \"сфы\": layout transcoding leaked through")
	}
	if _, _, ok := fp.fastFindMatch("сфыв"); !ok {
		t.Error("strict matcher rejected the exact substring \"сфы\"")
	}
}
