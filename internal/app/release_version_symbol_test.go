package app

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
)

// buildVersionAnchor is here for reflect: its package path is the one the
// linker needs in front of buildVersion.
type buildVersionAnchor struct{}

// Tag builds write the release tag into buildVersion with -X, and the linker
// skips an -X whose name resolves to nothing without a word. The variable once
// moved from package main to this package while the workflow went on naming
// main.buildVersion, so every tagged binary would have reported a commit hash
// and the updater would have offered each release as an update to itself.
// This test ties the two together: moving or renaming the variable breaks
// this file's build, and changing the name in the workflow fails the
// comparison below.
func TestReleaseWorkflowWritesBuildVersion(t *testing.T) {
	_ = buildVersion
	want := reflect.TypeOf(buildVersionAnchor{}).PkgPath() + ".buildVersion"

	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)

	decls := regexp.MustCompile(`(?m)^\s*VERSION_SYMBOL:\s*(\S+)\s*$`).FindAllStringSubmatch(workflow, -1)
	if len(decls) != 1 {
		t.Fatalf("build.yml declares VERSION_SYMBOL %d times, want once", len(decls))
	}
	if got := decls[0][1]; got != want {
		t.Fatalf("build.yml: VERSION_SYMBOL is %s, but the variable is %s", got, want)
	}

	// Every -X in the workflow goes through VERSION_SYMBOL, so a build step
	// added later cannot bring back a literal name that nothing checks.
	uses := regexp.MustCompile(`-X\s+"?([^\s="]+)=`).FindAllStringSubmatch(workflow, -1)
	if len(uses) == 0 {
		t.Fatal("no build step in build.yml writes the version")
	}
	for _, m := range uses {
		if m[1] != "$VERSION_SYMBOL" {
			t.Errorf("build.yml sets %s with -X; use $VERSION_SYMBOL", m[1])
		}
	}
}
