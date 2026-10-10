package vtui

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// gpuBackendModules are what the Ebitengine and gogpu backends bring in.
// vtui_noebiten and vtui_nogogpu exist so that a build keeping only the
// X11, Wayland and Win32 backends (f4's lite build) links none of them: not
// just never calls them, but never runs their package init either.
//
// goffi is not on the list: it is the FFI layer, and Wayland still needs it
// to load libxkbcommon.
var gpuBackendModules = []string{
	"github.com/hajimehoshi/ebiten/",
	"github.com/gogpu/",
	"github.com/go-webgpu/webgpu",
}

// noGPUBackendTags is the tag pair f4's lite build passes.
const noGPUBackendTags = "vtui_noebiten,vtui_nogogpu"

// vtuiDeps lists what this package links for goos/amd64, with or without
// noGPUBackendTags.
func vtuiDeps(t *testing.T, goos string, noGPU bool) []string {
	t.Helper()
	command := exec.Command("go", "list", "-deps", ".")
	if noGPU {
		command = exec.Command("go", "list", "-deps", "-tags", noGPUBackendTags, ".")
	}
	command.Env = append(os.Environ(), "GOOS="+goos, "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("GOOS=%s %s: %v\n%s", goos, strings.Join(command.Args, " "), err, stderr)
	}
	return strings.Fields(string(out))
}

func gpuBackendDeps(deps []string) []string {
	var found []string
	for _, dep := range deps {
		for _, prefix := range gpuBackendModules {
			if strings.HasPrefix(dep, prefix) {
				found = append(found, dep)
			}
		}
	}
	return found
}

func TestNoGPUBackendTagsDropEbitenAndGogpu(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	for _, goos := range []string{"linux", "windows", "darwin"} {
		if found := gpuBackendDeps(vtuiDeps(t, goos, true)); len(found) > 0 {
			t.Errorf("GOOS=%s -tags %s still depends on:\n\t%s", goos, noGPUBackendTags, strings.Join(found, "\n\t"))
		}
	}
}

// TestDefaultBuildKeepsEbitenAndGogpu keeps the test above honest: without
// the tags both backends are still there.
func TestDefaultBuildKeepsEbitenAndGogpu(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	deps := strings.Join(vtuiDeps(t, "linux", false), "\n")
	for _, want := range []string{"github.com/hajimehoshi/ebiten/v2\n", "github.com/gogpu/gogpu\n"} {
		if !strings.Contains(deps+"\n", want) {
			t.Errorf("a default linux/amd64 build no longer depends on %s", strings.TrimSpace(want))
		}
	}
}
