//go:build !lite

package netfox

import (
	"context"
	"github.com/unxed/f4/internal/netproxy"
	"testing"
)

func TestDialSSHFailsOnAClosedPort(t *testing.T) {
	client, err := DialSSH("127.0.0.1", "1", "nobody", "", "", 2, netproxy.Settings{})
	if err == nil {
		_ = client.Close() // unexpected dial cleanup only
		t.Fatal("dialing a closed port succeeded")
	}
}

func TestSSHFishDialerReportsAFailedDial(t *testing.T) {
	dial := sshFishDialer("127.0.0.1", deadTCPPort(t), "nobody", "", "", 1, netproxy.Settings{})
	stdin, stdout, closer, err := dial(context.Background())
	if err == nil {
		if closer != nil {
			_ = closer.Close() // unexpected dial cleanup only
		}
		t.Fatal("dialling a dead port succeeded")
	}
	if stdin != nil || stdout != nil || closer != nil {
		t.Fatal("a failed dial handed back something to use")
	}
}

func TestFullTransportCloseNilReceiver(t *testing.T) {
	var s *SFTPVFS
	if err := s.Close(); err != nil {
		t.Errorf("nil SFTPVFS.Close() returned error: %v", err)
	}
	var f *FTPVFS
	if err := f.Close(); err != nil {
		t.Errorf("nil FTPVFS.Close() returned error: %v", err)
	}
}
