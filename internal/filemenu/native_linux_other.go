//go:build linux && !(amd64 || arm64)

package filemenu

import "os/exec"

const Platform = "linux"

func showNative(r Request) Result {
	if r.Operation != "open" {
		return Result{Outcome: Unavailable}
	}
	for _, path := range r.Paths {
		if err := exec.Command("xdg-open", path).Run(); err != nil {
			return Result{Outcome: Failed, Error: err.Error()}
		}
	}
	return Result{Outcome: Invoked}
}
