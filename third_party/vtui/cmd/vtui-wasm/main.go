//go:build wasip1 || wasm

package main

import (
	"os"

	"github.com/unxed/vtui"
)

func main() {
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	session := vtui.NewProtocolSession(os.Stdin, os.Stdout, vtui.FrameManager)
	_ = session.Serve()
}
