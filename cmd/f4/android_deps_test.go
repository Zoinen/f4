package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// TestFullBuildExcludesAndroidPlugin is the mechanical half of f4#1178's
// Android extraction (part 1 of 4 of the plan at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645):
// plugins/android (ADB device browsing over shell-v2/FISH+ or ADB Sync) no
// longer links into f4 at all, in either the full or the lite build. It is
// its own module now (plugins/android/go.mod) built as a separate subprocess
// RPC plugin binary (plugins/android/cmd/android-plugin), which
// `go list -deps` on this module cannot even see, let alone report as a
// dependency -- but this test still asks the toolchain rather than trusting
// that fact to stay true, the same way TestFullBuildExcludesCloudDependencies
// (cloudfox_deps_test.go) does for cloudfox's own extraction.
//
// Unlike cloudfox, plugins/android carried no unique heavy third-party
// dependency of its own to begin with -- it is a thin wrapper around the
// local `adb` server/executable, reusing plugins/netfox's FISH+ machinery
// (github.com/unxed/f4/plugins/netfox, .../netfox/fishplus), which stays in
// the main module and is used in-process by both builds regardless of this
// extraction. So the forbidden list below is just the package path itself:
// there is no aws-sdk-go-v2-sized dependency tree to also check for, and
// grepping plugins/android/*.go's non-test imports confirms nothing beyond
// the standard library, github.com/unxed/f4/plugins/netfox(/fishplus),
// github.com/unxed/f4/sdk/f4plugin, github.com/unxed/f4/vfs and
// github.com/unxed/vtinput -- none of which are unique to plugins/android or
// forbidden elsewhere.
//
// This checks the *regular* (non-lite) build: plugins/android was never
// gated out of -tags lite in the first place (internal/plughost/manager.go
// registered it unconditionally in loadInternal, in both builds); what's new
// here is that plugins_full.go/manager.go no longer registers it in either
// build.
func TestFullBuildExcludesAndroidPlugin(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/unxed/f4/plugins/android",
	}

	command := exec.Command("go", "list", "-deps", "./cmd/f4")
	command.Dir = testutil.ModuleRootDir(t)
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -deps ./cmd/f4: %v\n%s", err, stderr)
	}
	deps := strings.Fields(string(out))

	var offenders []string
	for _, imported := range deps {
		for _, bad := range forbidden {
			if imported == bad || strings.HasPrefix(imported, bad+"/") {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a regular build of ./cmd/f4 still depends on:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}
