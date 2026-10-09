package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// TestFullBuildExcludesCloudDependencies is the mechanical half of f4#1178's
// CloudFox extraction (part 1 of 4 of the plan at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851218447):
// plugins/cloudfox (S3, Google Drive, Yandex Disk, WebDAV -- the ~30 MB of
// aws-sdk-go-v2, google.golang.org/api, golang.org/x/oauth2 and
// github.com/zalando/go-keyring) no longer links into f4 at all, in either
// the full or the lite build. It is its own module now
// (plugins/cloudfox/go.mod) built as a separate subprocess RPC plugin
// binary (plugins/cloudfox/cmd/cloudfox-plugin), which `go list -deps` on
// this module cannot even see, let alone report as a dependency -- but this
// test still asks the toolchain rather than trusting that fact to stay
// true, the same way TestLiteBuildExcludesHeavyNetworkDependencies does for
// netfox's FTP/SFTP/Pageant libraries.
//
// This checks the *regular* (non-lite) build deliberately: cloudfox was
// already absent from -tags lite before this change (f4#1469/#1477/#1486);
// what's new here is that plugins_full.go no longer registers it either.
func TestFullBuildExcludesCloudDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/aws/aws-sdk-go-v2",
		"github.com/aws/smithy-go",
		"github.com/zalando/go-keyring",
		"golang.org/x/oauth2",
		"google.golang.org/api",
		"github.com/unxed/f4/plugins/cloudfox",
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
