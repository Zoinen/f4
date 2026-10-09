//go:build !lite

package netfox

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/unxed/f4/vfs"
)

// smbURIProvider opens smb://[domain;]user[:password]@host[:port][/share[/path]]
// as a VFS (f4#188). Without a user the logon is anonymous.
type smbURIProvider struct{}

func (p *smbURIProvider) Scheme() string { return "smb" }

func (p *smbURIProvider) OpenURI(ctx context.Context, current vfs.VFS, raw string) (vfs.VFS, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("smb: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("smb: no host in %s", raw)
	}
	port := u.Port()
	if port == "" {
		port = "445"
	}
	domain, user, pass := "", "", ""
	if u.User != nil {
		user = u.User.Username()
		pass, _ = u.User.Password()
		// "DOMAIN;user", the form smbclient and Windows UNC shortcuts use.
		if d, name, ok := strings.Cut(user, ";"); ok {
			domain, user = d, name
		}
	}
	client, err := dialSMB(ctx, host, port, domain, user, pass)
	if err != nil {
		return nil, err
	}
	v := newSMBVFS(nil, client, host)
	if target := strings.TrimSpace(u.Path); target != "" && target != "/" {
		if err := v.SetPath(target); err != nil {
			_ = v.Close() // Preserve the invalid-path error.
			return nil, err
		}
	}
	return v, nil
}
