package netfox

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/unxed/f4/internal/netproxy"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestDialSSHAuthenticatesWithSSHAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSH_AUTH_SOCK", startTestSSHAgent(t, agent.AddedKey{PrivateKey: privateKey}))
	port, hostKey := startTestSSHServerWithPublicKey(t, sshPublicKeyFromEd25519(t, publicKey))
	writeKnownHosts(t, home, knownhosts.Normalize("127.0.0.1:"+port), hostKey)

	client, err := DialSSH("127.0.0.1", port, "agent-user", "", "", 3, netproxy.Settings{})
	if err != nil {
		t.Fatalf("SSH agent authentication failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("close SSH client: %v", err)
	}
}

func sshPublicKeyFromEd25519(t *testing.T, public ed25519.PublicKey) ssh.PublicKey {
	t.Helper()
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestSSHHostKeyCallbackAcceptsKnownKey(t *testing.T) {
	home := t.TempDir()
	key := testSSHHostKey(t)
	writeKnownHosts(t, home, "[example.test]:2222", key)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("example.test:2222", testSSHRemoteAddr{}, key); err != nil {
		t.Fatalf("known host key rejected: %v", err)
	}
}

func TestSSHHostKeyCallbackRejectsUnknownAndChangedKeys(t *testing.T) {
	home := t.TempDir()
	knownKey := testSSHHostKey(t)
	writeKnownHosts(t, home, "[example.test]:2222", knownKey)
	// An unknown host is now a question rather than a verdict; this test is
	// about what happens when the answer is no.
	stubHostKeyPrompt(t, false)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	unknownKey := testSSHHostKey(t)
	if err := callback("other.test:2222", testSSHRemoteAddr{}, unknownKey); err == nil {
		t.Fatal("unknown host key was accepted")
	}
	if err := callback("example.test:2222", testSSHRemoteAddr{}, unknownKey); err == nil {
		t.Fatal("changed host key was accepted")
	}
}

func TestSSHHostKeyCallbackReadsBothKnownHostsFiles(t *testing.T) {
	home := t.TempDir()
	key := testSSHHostKey(t)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := knownhosts.Line([]string{"[legacy.test]:2222"}, key) + "\n"
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts2"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	stubHostKeyPrompt(t, false)

	callback, err := sshHostKeyCallbackForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := callback("legacy.test:2222", testSSHRemoteAddr{}, key); err != nil {
		t.Fatalf("a key recorded in known_hosts2 was rejected: %v", err)
	}
}

func TestDialSSHVerifiesServerKeyBeforeAuthentication(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	port, publicKey := startTestSSHServer(t)
	writeKnownHosts(t, home, knownhosts.Normalize("127.0.0.1:"+port), publicKey)

	client, err := DialSSH("127.0.0.1", port, "user", "pass", "", 3, netproxy.Settings{})
	if err != nil {
		t.Fatalf("known server key rejected: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("close SSH client: %v", err)
	}
}

func TestDialSSHRejectsChangedServerKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	port, _ := startTestSSHServer(t)
	// The test server carries an RSA key, so an RSA entry that does not match
	// it is a replaced key rather than an unrecorded algorithm. A user who
	// would say yes to anything must not get the chance.
	writeKnownHosts(t, home, knownhosts.Normalize("127.0.0.1:"+port), testSSHRSAHostKey(t))
	prompt := stubHostKeyPrompt(t, true)

	client, err := DialSSH("127.0.0.1", port, "user", "pass", "", 3, netproxy.Settings{})
	if err == nil {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("close SSH client: %v", closeErr)
		}
		t.Fatal("changed server key was accepted")
	}
	if asked := prompt.questions(); len(asked) != 0 {
		t.Fatalf("a changed server key raised %d trust questions, want none", len(asked))
	}
}

func TestDialSSHRecordsAServerKeyTheUserTrusts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SSH_AUTH_SOCK", "")
	port, publicKey := startTestSSHServer(t)
	prompt := stubHostKeyPrompt(t, true)

	// No known_hosts anywhere: the state a fresh account is in, and the one
	// that used to fail the connection outright.
	client, err := DialSSH("127.0.0.1", port, "user", "pass", "", 3, netproxy.Settings{})
	if err != nil {
		t.Fatalf("a trusted first connection failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("close SSH client: %v", err)
	}
	if asked := prompt.questions(); len(asked) != 1 {
		t.Fatalf("trust questions asked = %d, want 1", len(asked))
	}

	// The recorded key is what makes the second connection silent.
	callback, err := knownhosts.New(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		t.Fatalf("known_hosts was not written: %v", err)
	}
	if err := callback("127.0.0.1:"+port, testSSHRemoteAddr{}, publicKey); err != nil {
		t.Fatalf("the recorded server key does not verify: %v", err)
	}
}

func testSSHHostKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func writeKnownHosts(t *testing.T, home, address string, key ssh.PublicKey) {
	t.Helper()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sshDir, "known_hosts")
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{address}, key)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func startTestSSHServer(t *testing.T) (string, ssh.PublicKey) {
	return startTestSSHServerWithPublicKey(t, nil)
}

func startTestSSHServerWithPublicKey(t *testing.T, authKey ssh.PublicKey) (string, ssh.PublicKey) {
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
	if authKey != nil {
		config.PublicKeyCallback = func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() == "agent-user" && bytes.Equal(key.Marshal(), authKey.Marshal()) {
				return nil, nil
			}
			return nil, fmt.Errorf("test SSH server rejected public key")
		}
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close test SSH listener: %v", err)
		}
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				serverConn, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					_ = conn.Close()
					return
				}
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					_ = channel.Reject(ssh.Prohibited, "test server")
				}
				_ = serverConn.Close()
			}()
		}
	}()
	port := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
	return port, signer.PublicKey()
}

type testSSHRemoteAddr struct{}

func (testSSHRemoteAddr) Network() string { return "tcp" }
func (testSSHRemoteAddr) String() string  { return "127.0.0.1:2222" }

var _ net.Addr = testSSHRemoteAddr{}
