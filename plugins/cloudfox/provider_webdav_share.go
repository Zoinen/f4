package cloudfox

import (
	"context"
	"fmt"
	"strings"

	"github.com/unxed/f4/vfs"
)

const webDAVShareNotice = "This is the direct WebDAV resource address, not a public share link. It contains no CloudFox credentials; whether browser GET is public or requires authentication is determined by the server's resource- and method-specific ACLs. Public token links, passwords, expiration, and per-link permissions are not defined by Generic WebDAV."

func (b *webDAVBackend) ShareLinkInfo(ctx context.Context, location string) (vfs.ShareLinkInfo, error) {
	if err := ctx.Err(); err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	entry, err := b.Stat(ctx, location)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	target, err := b.urlFor(location)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	if entry.IsDir && !strings.HasSuffix(target.Path, "/") {
		target.Path += "/"
		target.RawPath = ""
	}
	link := vfs.ShareLink{
		URL:       target.String(),
		Role:      vfs.ShareRoleServerControlled,
		Revocable: false,
	}
	return vfs.ShareLinkInfo{
		Provider:  "Generic WebDAV",
		ItemName:  entry.Name,
		Roles:     []vfs.ShareRole{link.Role},
		CanCreate: false,
		CanRevoke: false,
		Link:      &link,
		Notice:    webDAVShareNotice,
	}, nil
}

func (*webDAVBackend) CreateShareLink(ctx context.Context, _ string, _ vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	if err := ctx.Err(); err != nil {
		return vfs.ShareLink{}, err
	}
	return vfs.ShareLink{}, fmt.Errorf("%w: Generic WebDAV has no standard public-link creation API", ErrShareLinksUnsupported)
}

func (*webDAVBackend) RevokeShareLink(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("%w: Generic WebDAV has no standard public-link revocation API", ErrShareLinksUnsupported)
}

var _ BackendShareLinker = (*webDAVBackend)(nil)
