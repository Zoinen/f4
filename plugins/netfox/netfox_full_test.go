//go:build !lite

package netfox

import (
	"bytes"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
	"net"
	"testing"
	"time"
)

func TestNetFox_TimeoutAndDial(t *testing.T) {
	// 1. Start a local mock TCP server that does NOT send the FTP greeting (simulating wrong protocol)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock TCP server: %v", err)
	}
	defer func() {
		_ = l.Close() // listener cleanup only
	}()

	addr := l.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	// Accept connection in background but do nothing (simulating hang)
	go func() {
		conn, err := l.Accept()
		if err == nil {
			defer func() {
				_ = conn.Close() // connection cleanup only
			}()
			time.Sleep(2 * time.Second)
		}
	}()

	// 2. Attempt to connect using FTPVFS with a very short 1-second timeout
	start := time.Now()
	_, err = NewFTPVFS(nil, host, port, "user", "pass", 1, nil, "", netproxy.Settings{})
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected connection to fail due to timeout, but it succeeded")
	}

	// The connection should fail and return within approx 1 second (plus small buffer), not hang
	if duration > 1500*time.Millisecond {
		t.Errorf("Timeout took too long: %v (expected ~1s)", duration)
	}
}

func TestNetFox_CodepageSupport(t *testing.T) {
	v := &FTPVFS{
		cwd: "/home",
	}
	dec, enc := vfs.GetCodepageDecoderEncoder("1251")
	v.decoder = dec
	v.encoder = enc

	encoded := v.encodePath("Привет")
	expected := []byte{0xcf, 0xf0, 0xe8, 0xe2, 0xe5, 0xf2}
	if !bytes.Equal([]byte(encoded), expected) {
		t.Errorf("encodePath failed: expected bytes %v, got %q", expected, []byte(encoded))
	}
}
