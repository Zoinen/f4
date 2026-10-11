//go:build windows

package dockerfs

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/windows"
)

// TestNamedPipeEndToEnd talks to a real Windows named pipe: the server side is
// made with CreateNamedPipe and answers one request, the client side is the
// code a DOCKER_HOST of npipe:// uses.
func TestNamedPipeEndToEnd(t *testing.T) {
	name := `\\.\pipe\f4-dockerfs-test-` + strconv.Itoa(os.Getpid())
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateNamedPipe(namePtr, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT, 1, 65536, 65536, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := windows.ConnectNamedPipe(h, nil); err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			_ = windows.CloseHandle(h)
			return
		}
		f := os.NewFile(uintptr(h), name)
		defer func() { _ = f.Close() }()
		if _, err := http.ReadRequest(bufio.NewReader(f)); err != nil {
			return
		}
		body := `[{"Id":"aaaaaaaaaaaaaaaa","Names":["/web"]}]`
		_, _ = io.WriteString(f, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nConnection: close\r\n\r\n"+body)
		_ = f.Sync() // FlushFileBuffers: wait until the client has read it
	}()

	cli, err := newClient("npipe:////./pipe/f4-dockerfs-test-" + strconv.Itoa(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	list, err := cli.listContainers(context.Background())
	if err != nil || len(list) != 1 || list[0].name() != "web" {
		t.Fatalf("over a real named pipe: %+v, %v", list, err)
	}
	<-done
}
