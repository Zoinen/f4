package dialog

import (
	"github.com/unxed/f4/internal/testutil"
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAndLangCompleteness(t *testing.T) {
	langs, err := filepath.Glob(filepath.Join(testutil.ModuleRootDir(t), "internal", "i18n", "lang", "*.lng"))
	if err != nil {
		t.Fatal(err)
	}
	helps, err := filepath.Glob("help/*.hlf")
	if err != nil {
		t.Fatal(err)
	}
	// Glob reports no error for zero matches: a wrong directory would leave
	// this comparison passing over two empty sets.
	if len(langs) == 0 || len(helps) == 0 {
		t.Fatalf("found %d .lng and %d .hlf files, want both non-empty", len(langs), len(helps))
	}

	langSet := make(map[string]bool)
	for _, l := range langs {
		base := filepath.Base(l)
		langSet[strings.TrimSuffix(base, ".lng")] = true
	}

	helpSet := make(map[string]bool)
	for _, h := range helps {
		base := filepath.Base(h)
		helpSet[strings.TrimSuffix(base, ".hlf")] = true
	}

	for l := range langSet {
		if !helpSet[l] {
			t.Errorf("Language %q has a .lng file but is missing a corresponding .hlf help file.", l)
		}
	}

	for h := range helpSet {
		if !langSet[h] {
			t.Errorf("Language %q has a .hlf help file but is missing a corresponding .lng file.", h)
		}
	}
}
