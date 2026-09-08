//go:build linux && (amd64 || arm64)

package main

import (
	"slices"
	"testing"
)

// The list of variables the terminal strips from a child's environment is
// spelled out where every platform can read it, while the code that reads
// those variables is here. Nothing but this test keeps the two in step: a
// rename on either side would leave the terminal stripping a name nobody
// sets, and the child would go back to dying before main (issue #87).
func TestPrivateEnvCoversWhatThisBuildReads(t *testing.T) {
	for _, key := range []string{goffiUniversalGuard, goffiUniversalExe, f4ExeEnv} {
		if !slices.Contains(privateToThisProcess, key) {
			t.Errorf("%s is read here but still passed on to children", key)
		}
	}
}
