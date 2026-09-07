package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/tools/hardcode"
)

const hardcodedBaselinePath = "tools/hardcoded_baseline.txt"

const hardcodedUpdateHint = "Regenerate it with:\n" +
	"    F4_UPDATE_HARDCODED_BASELINE=1 go test -run TestNoNewHardcodedUIStrings ."

// TestNoNewHardcodedUIStrings is the CI gate described in L10N_PLAN.md, stage
// S0. Captions that already existed when the localization audit started are
// frozen in tools/hardcoded_baseline.txt; that list may only shrink.
func TestNoNewHardcodedUIStrings(t *testing.T) {
	root := moduleRootDir(t)
	baselinePath := filepath.Join(root, hardcodedBaselinePath)
	findings, err := hardcode.Scan(root)
	if err != nil {
		t.Fatalf("hardcoded string scan failed: %v", err)
	}

	if os.Getenv("F4_UPDATE_HARDCODED_BASELINE") == "1" {
		if err := hardcode.WriteBaseline(baselinePath, findings); err != nil {
			t.Fatalf("cannot update %s: %v", hardcodedBaselinePath, err)
		}
		t.Logf("%s regenerated with %d entries, commit it", hardcodedBaselinePath, len(hardcode.IDs(findings)))
		return
	}

	baseline, err := hardcode.LoadBaseline(baselinePath)
	if os.IsNotExist(err) {
		if werr := hardcode.WriteBaseline(baselinePath, findings); werr != nil {
			t.Fatalf("cannot create %s: %v", hardcodedBaselinePath, werr)
		}
		t.Logf("%s did not exist and was created with %d entries, commit it", hardcodedBaselinePath, len(hardcode.IDs(findings)))
		return
	}
	if err != nil {
		t.Fatalf("cannot read %s: %v", hardcodedBaselinePath, err)
	}

	seen := make(map[string]bool, len(findings))
	var added []string
	for _, f := range findings {
		id := f.ID()
		seen[id] = true
		if !baseline[id] {
			added = append(added, f.String())
		}
	}

	if len(added) > 0 {
		t.Errorf("%d UI caption(s) are hardcoded instead of coming from lang/en.lng:\n%s\n\n"+
			"Add a key to lang/en.lng and wrap the caption in Msg(). See I18N.md and L10N_PLAN.md.",
			len(added), strings.Join(added, "\n"))
	}

	var stale []string
	for id := range baseline {
		if !seen[id] {
			stale = append(stale, id)
		}
	}
	if len(stale) > 0 {
		t.Errorf("%d baseline entry(ies) in %s no longer exist. That is good news: the list must be shrunk to match.\n%s",
			len(stale), hardcodedBaselinePath, hardcodedUpdateHint)
	}
}
