package app

import (
	"fmt"
	"io"

	"github.com/unxed/f4/plugins/netfox/fishplus"
)

// runFishServer serves one FISH+ session over in and out and returns the exit
// code of the process: 0 when the client ended the session or closed the
// stream, 1 when the stream could not be followed. It is what a client starts
// as the remote command of a connection to a host that has f4 installed,
// instead of uploading the shell helper (unxed/f4#1680).
func runFishServer(in io.Reader, out, errOut io.Writer) int {
	if err := (&fishplus.Server{}).Serve(in, out); err != nil {
		_, _ = fmt.Fprintf(errOut, "f4 --fish-server: %v\n", err)
		return 1
	}
	return 0
}
