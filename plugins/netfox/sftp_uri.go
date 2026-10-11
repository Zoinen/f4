//go:build !lite

// Calls NewSFTPVFS (sftp_vfs.go), which is what pulls this file under that
// file's own -tags lite exclusion -- see its comment for why.

package netfox

import (
	"context"
	"fmt"
	"net/url"
	"os/user"
	"strings"

	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
)

// sftpURIProvider opens sftp://[user@]host[:port]/path as a VFS.
//
// Until now an SFTP connection could only be opened from a stored NetFox
// configuration, which is fine for the panels and useless everywhere else: a
// mount command, an fstab line and a benchmark all have nothing but a string.
// This is that string.
//
// Credentials are deliberately limited to what a non-interactive caller can
// supply: the key material ssh would use anyway, or a password embedded in the
// URL. A connection that would need to ask fails instead of hanging, which is
// what FUSE.md requires of anything a --daemon mount can reach.
//
// The same provider also serves scp://[user@]host[:port]/path (f4#187). SCP
// has no directory listing of its own -- a client has to run ls on the far
// side, which is what FISH+ already does -- and every server that speaks SCP
// today also speaks SFTP, so a scp:// address opens the SFTP backend, which
// lists, reads, writes and renames where a bare SCP transfer could only copy
// a whole file.
type sftpURIProvider struct {
	// alias is the scheme this instance answers to instead of "sftp".
	alias string
}

func (p *sftpURIProvider) Scheme() string {
	if p.alias != "" {
		return p.alias
	}
	return "sftp"
}

func (p *sftpURIProvider) OpenURI(ctx context.Context, current vfs.VFS, raw string) (vfs.VFS, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", p.Scheme(), err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("%s: no host in %s", p.Scheme(), raw)
	}
	port := u.Port()
	if port == "" {
		port = "22"
	}

	name := ""
	pass := ""
	if u.User != nil {
		name = u.User.Username()
		pass, _ = u.User.Password()
	}
	if name == "" {
		// The same default ssh uses when the URL does not say.
		if me, err := user.Current(); err == nil {
			name = me.Username
		}
	}

	v, err := NewSFTPVFS(nil, host, port, name, pass, "", 15, "", netproxy.Resolve(netproxy.Settings{}))
	if err != nil {
		return nil, err
	}
	if p := strings.TrimSpace(u.Path); p != "" && p != "/" {
		if err := v.SetPath(p); err != nil {
			_ = v.Close() // Preserve the invalid-path error.
			return nil, err
		}
	}
	return v, nil
}
