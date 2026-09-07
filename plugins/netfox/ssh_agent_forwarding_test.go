package netfox

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/unxed/f4/internal/netproxy"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestDialSSHDoesNotForwardAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSH_AUTH_SOCK", startTestSSHAgent(t))
	port, publicKey, forwarded := startAgentObservationSSHServer(t)
	writeKnownHosts(t, home, knownhosts.Normalize("127.0.0.1:"+port), publicKey)

	client, err := DialSSH("127.0.0.1", port, "user", "pass", "", 3, netproxy.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("close SSH client: %v", err)
	}
	assertAgentNotForwarded(t, forwarded)
}

func TestSSHFishDialerDoesNotRequestAgentForwarding(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSH_AUTH_SOCK", "agent-present")
	port, publicKey, forwarded := startAgentObservationSSHServer(t)
	writeKnownHosts(t, home, knownhosts.Normalize("127.0.0.1:"+port), publicKey)
	dial := sshFishDialerWith("127.0.0.1", port, "user", "pass", "", 3, netproxy.Settings{}, func(session *ssh.Session) error {
		return session.Shell()
	})

	_, _, closer, err := dial(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Errorf("close FISH+ shell: %v", err)
	}
	assertAgentNotForwarded(t, forwarded)
}

func assertAgentNotForwarded(t *testing.T, forwarded <-chan struct{}) {
	t.Helper()
	select {
	case <-forwarded:
		t.Fatal("SSH agent forwarding was requested")
	case <-time.After(250 * time.Millisecond):
	}
}

func startTestSSHAgent(t *testing.T, keys ...agent.AddedKey) string {
	t.Helper()
	// Darwin has a much shorter limit for Unix socket paths than Linux. Keep
	// the socket directly under the system temp directory instead of nesting
	// it under t.TempDir(), whose randomized path can exceed that limit.
	path := filepath.Join(os.TempDir(), fmt.Sprintf("f4-agent-%d.sock", os.Getpid()))
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen for test SSH agent: %v", err)
	}
	keyring := agent.NewKeyring()
	for _, key := range keys {
		if err := keyring.Add(key); err != nil {
			t.Fatalf("add key to test SSH agent: %v", err)
		}
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close test SSH agent listener: %v", err)
		}
		_ = os.Remove(path)
	})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_ = agent.ServeAgent(keyring, conn)
		_ = conn.Close()
	}()
	return path
}

func startAgentObservationSSHServer(t *testing.T) (string, ssh.PublicKey, <-chan struct{}) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() == "user" && string(password) == "pass" {
				return nil, nil
			}
			return nil, fmt.Errorf("test SSH server rejected credentials")
		},
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	forwarded := make(chan struct{}, 1)
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close test SSH server listener: %v", err)
		}
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go observeAgentForwarding(conn, config, forwarded)
		}
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	return port, signer.PublicKey(), forwarded
}

func observeAgentForwarding(conn net.Conn, config *ssh.ServerConfig, forwarded chan<- struct{}) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	go func() {
		for request := range requests {
			if request.Type == "auth-agent-req@openssh.com" {
				select {
				case forwarded <- struct{}{}:
				default:
				}
			}
			_ = request.Reply(true, nil)
		}
	}()
	for newChannel := range channels {
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go func() {
			for request := range channelRequests {
				_ = request.Reply(true, nil)
			}
			_ = channel.Close()
		}()
	}
	_ = serverConn.Close()
}
