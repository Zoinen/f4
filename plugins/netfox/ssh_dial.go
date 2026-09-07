package netfox

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unxed/f4/internal/netproxy"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// sshAgentReadWriter deliberately exposes only Read and Write. This makes
// x/crypto/ssh/agent use its serialized client mode instead of starting a
// background reader. The native Pageant transport is a request/response
// connection whose Read returns EOF between requests, unlike a Unix socket;
// treating it as a streaming io.ReadWriteCloser would make the first agent
// response terminate the client before the SSH signature request.
type sshAgentReadWriter struct{ io.ReadWriter }

func newSSHAgentClient(rw io.ReadWriter) agent.ExtendedAgent {
	return agent.NewClient(sshAgentReadWriter{ReadWriter: rw})
}

// sshTimeout turns the timeout a site configuration carries into a duration,
// falling back to something sane when the field is empty or nonsense.
func sshTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

// expandHome turns a leading ~ (or ~/ or ~\) into the user's home directory.
// Go's os package never does this on its own — that expansion is normally
// the shell's job — but a path typed into the connection dialog has no
// shell behind it, so a bare ~/.ssh/key would otherwise resolve to a
// nonexistent file named literally "~" in the working directory.
func expandHome(p string) string {
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// loadKeySigner reads a private key file and returns a Signer for it. If the
// key is encrypted, pass is tried as its passphrase.
func loadKeySigner(keyPath, pass string) (ssh.Signer, error) {
	keyBytes, err := os.ReadFile(expandHome(keyPath))
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil && pass != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(pass))
	}
	return signer, err
}

// DialSSH opens an SSH connection the way every SSH based NetFox backend
// needs it. When keyPath is set, that key is the only key offered (besides
// whatever the ssh-agent carries) — this avoids servers that cut the
// handshake short after a handful of failed public-key attempts
// (MaxAuthTries) before ever reaching the right key. When keyPath is empty,
// behavior is unchanged: agent, then the usual private keys from ~/.ssh,
// then the password. It is shared by the SFTP and the FISH+ backends so
// that a site behaves identically whichever of them opens it.
func DialSSH(host, port, user, pass, keyPath string, timeout int, px netproxy.Settings) (*ssh.Client, error) {
	hostKeyCallback, err := sshHostKeyCallback()
	if err != nil {
		return nil, err
	}

	auths := []ssh.AuthMethod{}
	var agentConn io.ReadWriteCloser

	if conn, err := openSSHAgent(); err == nil {
		agentConn = conn
		agentClient := newSSHAgentClient(conn)
		auths = append(auths, ssh.PublicKeysCallback(agentClient.Signers))
	}

	if keyPath != "" {
		if signer, err := loadKeySigner(keyPath, pass); err == nil {
			auths = append(auths, ssh.PublicKeys(signer))
		}
	} else {
		home, _ := os.UserHomeDir()
		for _, keyName := range []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"} {
			defaultKeyPath := filepath.Join(home, ".ssh", keyName)
			if signer, err := loadKeySigner(defaultKeyPath, pass); err == nil {
				auths = append(auths, ssh.PublicKeys(signer))
			}
		}
	}

	if pass != "" {
		auths = append(auths, ssh.Password(pass))
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            auths,
		HostKeyCallback: hostKeyCallback,
		Timeout:         sshTimeout(timeout),
	}
	// ssh.Dial would open the socket itself; going through netproxy instead
	// is what lets a site sit behind an HTTP CONNECT or SOCKS5 gateway.
	client, err := dialSSHVia(px, host+":"+port, config)
	if err != nil {
		if agentConn != nil {
			_ = agentConn.Close() // Preserve the SSH dial failure.
		}
		return nil, err
	}
	if agentConn != nil {
		// The agent is used only for local authentication. Keeping its socket
		// open after the SSH handshake would make forwarding tempting and would
		// hold a needless connection to the user's agent for the whole session.
		_ = agentConn.Close()
	}
	return client, nil
}

func sshHostKeyCallback() (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("SSH host-key verification: determine home directory: %w", err)
	}
	return sshHostKeyCallbackForHome(home)
}

func sshHostKeyCallbackForHome(home string) (ssh.HostKeyCallback, error) {
	knownHosts, err := newSSHKnownHosts(home)
	if err != nil {
		return nil, err
	}
	return knownHosts.check, nil
}

// dialSSHVia opens the transport through px and speaks SSH over it.
func dialSSHVia(px netproxy.Settings, addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	ctx := context.Background()
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}
	conn, err := px.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if config.Timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(config.Timeout))
		config = withHostKeyPromptDeadline(conn, config)
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close() // Preserve the SSH handshake failure.
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// withHostKeyPromptDeadline copies config so that its host-key callback runs
// with the dial deadline lifted. The callback is where an unknown host asks
// the user whether to trust it, and that question waits for a human: the
// fifteen second deadline the dial arms would otherwise expire under the
// dialog and turn every answer, including yes, into a handshake failure. The
// deadline is rearmed on the way out, so the rest of the handshake stays
// bounded.
func withHostKeyPromptDeadline(conn net.Conn, config *ssh.ClientConfig) *ssh.ClientConfig {
	verify := config.HostKeyCallback
	if verify == nil {
		return config
	}
	timeout := config.Timeout
	relaxed := *config
	relaxed.HostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		_ = conn.SetDeadline(time.Time{})
		defer func() { _ = conn.SetDeadline(time.Now().Add(timeout)) }()
		return verify(hostname, remote, key)
	}
	return &relaxed
}
