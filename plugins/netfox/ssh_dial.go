package netfox

import (
	"github.com/unxed/vtui"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"net"
	"os"
	"path/filepath"
	"time"
)

// sshTimeout turns the timeout a site configuration carries into a duration,
// falling back to something sane when the field is empty or nonsense.
func sshTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return 15 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

// DialSSH opens an SSH connection the way every SSH based NetFox backend
// needs it: the agent first, then the usual private keys from ~/.ssh, then
// the password. It is shared by the SFTP and the FISH+ backends so that a
// site behaves identically whichever of them opens it.
func DialSSH(host, port, user, pass string, timeout int) (*ssh.Client, error) {
	auths := []ssh.AuthMethod{}
	var agentClient agent.Agent
	var agentConn net.Conn

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			agentConn = conn
			agentClient = agent.NewClient(conn)
			auths = append(auths, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	home, _ := os.UserHomeDir()
	for _, keyName := range []string{"id_rsa", "id_ed25519", "id_ecdsa", "id_dsa"} {
		keyPath := filepath.Join(home, ".ssh", keyName)
		if keyBytes, err := os.ReadFile(keyPath); err == nil {
			signer, err := ssh.ParsePrivateKey(keyBytes)
			if err != nil && pass != "" {
				signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(pass))
			}
			if err == nil {
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
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         sshTimeout(timeout),
	}
	client, err := ssh.Dial("tcp", host+":"+port, config)
	if err != nil {
		if agentConn != nil {
			agentConn.Close()
		}
		return nil, err
	}
	if agentClient != nil {
		if err := agent.ForwardToAgent(client, agentClient); err != nil {
			vtui.DebugLog("SSH: Failed to forward agent: %v", err)
			agentConn.Close()
		} else {
			vtui.DebugLog("SSH: Agent forwarding enabled")
		}
	}
	return client, nil
}
