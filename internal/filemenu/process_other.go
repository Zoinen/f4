//go:build !windows

package filemenu

import (
	"context"
	"io"
	"os/exec"
)

func runPlatform(ctx context.Context, r Request) Result { return runOnce(ctx, r) }
func Serve(in io.Reader, out io.Writer) int             { return serveOnce(in, out) }
func EnablePreparation()                                {}
func Prepare(paths []string)                            {}
func Close()                                            {}

func configureProcess(cmd *exec.Cmd) {}

func prepareProcessRequest(cmd *exec.Cmd, r Request) {}
