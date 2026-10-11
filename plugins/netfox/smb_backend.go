//go:build !lite

package netfox

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/cloudsoda/go-smb2"
)

// smbClient is the smbBackend over a real SMB2/3 connection (go-smb2): one
// TCP connection, one authenticated session, and one tree connect per share
// the user has gone into.
type smbClient struct {
	mu      sync.Mutex
	conn    net.Conn
	session *smb2.Session
	shares  map[string]*smb2.Share
}

// dialSMB connects to host:port and authenticates with NTLM. An empty user
// means the guest/anonymous logon.
func dialSMB(ctx context.Context, host, port, domain, user, pass string) (*smbClient, error) {
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	d := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: user, Password: pass, Domain: domain}}
	session, err := d.DialConn(ctx, conn, net.JoinHostPort(host, port))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("smb: %w", err)
	}
	return &smbClient{conn: conn, session: session, shares: make(map[string]*smb2.Share)}, nil
}

// backendPath turns "a/b" into the backslash form SMB uses inside a share.
func backendPath(rel string) string { return strings.ReplaceAll(rel, "/", `\`) }

func (c *smbClient) share(name string) (*smb2.Share, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s, ok := c.shares[name]; ok {
		return s, nil
	}
	s, err := c.session.Mount(name)
	if err != nil {
		return nil, err
	}
	c.shares[name] = s
	return s, nil
}

func (c *smbClient) ListShares() ([]string, error) { return c.session.ListSharenames() }

func (c *smbClient) ReadDir(share, dir string) ([]fs.FileInfo, error) {
	s, err := c.share(share)
	if err != nil {
		return nil, err
	}
	return s.ReadDir(backendPath(dir))
}

func (c *smbClient) Stat(share, name string) (fs.FileInfo, error) {
	s, err := c.share(share)
	if err != nil {
		return nil, err
	}
	return s.Stat(backendPath(name))
}

type smbRemoteFile struct {
	*smb2.File
	size int64
}

func (f smbRemoteFile) Size() int64 { return f.size }

func (c *smbClient) OpenRead(share, name string) (smbFile, error) {
	s, err := c.share(share)
	if err != nil {
		return nil, err
	}
	f, err := s.Open(backendPath(name))
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return smbRemoteFile{File: f, size: info.Size()}, nil
}

func (c *smbClient) MkDir(share, dir string) error {
	s, err := c.share(share)
	if err != nil {
		return err
	}
	return s.Mkdir(backendPath(dir), 0o755)
}

func (c *smbClient) RemoveAll(share, name string) error {
	s, err := c.share(share)
	if err != nil {
		return err
	}
	return s.RemoveAll(backendPath(name))
}

func (c *smbClient) Rename(share, oldName, newName string) error {
	s, err := c.share(share)
	if err != nil {
		return err
	}
	return s.Rename(backendPath(oldName), backendPath(newName))
}

func (c *smbClient) Create(share, name string) (io.WriteCloser, error) {
	s, err := c.share(share)
	if err != nil {
		return nil, err
	}
	return s.Create(backendPath(name))
}

func (c *smbClient) Close() error {
	c.mu.Lock()
	shares := c.shares
	c.shares = make(map[string]*smb2.Share)
	c.mu.Unlock()
	for _, s := range shares {
		_ = s.Umount()
	}
	_ = c.session.Logoff()
	return c.conn.Close()
}
