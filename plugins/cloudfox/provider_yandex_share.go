package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const yandexShareNotice = "Yandex.Disk supports public viewer links only; link expiration and editing are not available through its REST API."

const yandexShareRollbackTimeout = 5 * time.Second

// yandexShareGate serializes the metadata/publish/metadata cycle with link
// inspection and revocation. A caller canceled while queued can still return
// without waiting for the active sharing operation to finish.
type yandexShareGate struct {
	once sync.Once
	ch   chan struct{}
}

func (gate *yandexShareGate) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	gate.once.Do(func() {
		gate.ch = make(chan struct{}, 1)
		gate.ch <- struct{}{}
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-gate.ch:
		return nil
	}
}

func (gate *yandexShareGate) unlock() { gate.ch <- struct{}{} }

type yandexShareResource struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	PublicURL string `json:"public_url"`
}

func (b *yandexDiskBackend) shareLocation(location string) (string, error) {
	location, err := b.Normalize(location)
	if err != nil {
		return "", err
	}
	// Publishing the configured root would expose much more than the selected
	// file or folder and is not an operation CloudFox offers in its item dialog.
	if b.IsRoot(location) {
		return "", ErrShareLinksUnsupported
	}
	return location, nil
}

func (b *yandexDiskBackend) shareResource(ctx context.Context, location string) (yandexShareResource, error) {
	query := url.Values{
		"path":   {location},
		"fields": {"name,public_key,public_url"},
	}
	resp, err := b.apiRequest(ctx, http.MethodGet, "/resources", query, nil)
	if err != nil {
		return yandexShareResource{}, err
	}
	defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// A public URL is a bearer-like credential. Do not include an arbitrary
		// provider response body in an error which can reach logs or history.
		return yandexShareResource{}, mapProviderHTTPError(resp, "")
	}
	var resource yandexShareResource
	if err := json.NewDecoder(resp.Body).Decode(&resource); err != nil {
		return yandexShareResource{}, fmt.Errorf("cloudfox: decode Yandex.Disk share metadata: %w", err)
	}
	if resource.PublicURL != "" {
		if err := validateHTTPSShareURL(resource.PublicURL); err != nil {
			return yandexShareResource{}, err
		}
	}
	return resource, nil
}

func (b *yandexDiskBackend) shareMutation(ctx context.Context, location, endpoint, operation string) error {
	resp, err := b.apiRequest(ctx, http.MethodPut, endpoint, url.Values{"path": {location}}, nil)
	if err != nil {
		return &vfs.UnknownOperationStateError{Operation: operation, Err: err}
	}
	defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
	// Yandex documents publish and unpublish as synchronous operations which
	// return 200 and a metadata Link. The Link is deliberately ignored: it is
	// not the public URL, and following an API-provided URL would add an SSRF and
	// credential-forwarding surface. Fetch metadata through our configured API
	// origin after publication instead.
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	return providerHTTPMutationError(operation, resp, "")
}

func yandexShareInfo(resource yandexShareResource) vfs.ShareLinkInfo {
	info := vfs.ShareLinkInfo{
		Provider:          "Yandex.Disk",
		ItemName:          resource.Name,
		Roles:             []vfs.ShareRole{vfs.ShareRoleViewer},
		ExpirationOptions: []time.Duration{0},
		CanCreate:         true,
		Notice:            yandexShareNotice,
	}
	if resource.PublicURL != "" {
		info.CanRevoke = true
		info.Link = &vfs.ShareLink{
			URL:       resource.PublicURL,
			Role:      vfs.ShareRoleViewer,
			Revocable: true,
		}
	}
	return info
}

func yandexShareLink(resource yandexShareResource) (vfs.ShareLink, bool, error) {
	if resource.PublicKey != "" && resource.PublicURL == "" {
		return vfs.ShareLink{}, false, errors.New("cloudfox: Yandex.Disk returned incomplete share metadata")
	}
	if resource.PublicURL == "" {
		return vfs.ShareLink{}, false, nil
	}
	return vfs.ShareLink{
		URL:       resource.PublicURL,
		Role:      vfs.ShareRoleViewer,
		Revocable: true,
	}, true, nil
}

func yandexShareRollbackUnknown(original error) error {
	cause := errors.New("cloudfox: Yandex.Disk could not confirm share-link rollback")
	// Preserve cancellation identity for callers while deliberately omitting
	// provider errors and returned metadata, either of which may contain a
	// bearer-like public URL.
	if errors.Is(original, context.Canceled) {
		cause = fmt.Errorf("%w: Yandex.Disk could not confirm share-link rollback", context.Canceled)
	} else if errors.Is(original, context.DeadlineExceeded) {
		cause = fmt.Errorf("%w: Yandex.Disk could not confirm share-link rollback", context.DeadlineExceeded)
	}
	return &vfs.UnknownOperationStateError{
		Operation: "Yandex.Disk publish share-link rollback",
		Err:       cause,
	}
}

func (b *yandexDiskBackend) rollbackPublishedShare(ctx context.Context, location string, original error) error {
	rollbackCtx, cancel := providerDetachedCleanupContext(ctx, yandexShareRollbackTimeout)
	defer cancel()
	if err := b.shareMutation(rollbackCtx, location, "/resources/unpublish", "Yandex.Disk rollback published share link"); err != nil {
		return yandexShareRollbackUnknown(original)
	}
	return original
}

func (b *yandexDiskBackend) ShareLinkInfo(ctx context.Context, location string) (vfs.ShareLinkInfo, error) {
	location, err := b.shareLocation(location)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	if err := b.shareGate.lock(ctx); err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	defer b.shareGate.unlock()
	resource, err := b.shareResource(ctx, location)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	if resource.Name == "" {
		resource.Name = b.Base(location)
	}
	if _, _, err := yandexShareLink(resource); err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	return yandexShareInfo(resource), nil
}

func (b *yandexDiskBackend) CreateShareLink(ctx context.Context, location string, request vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	if request.Role != vfs.ShareRoleViewer || request.ExpiresIn != 0 {
		return vfs.ShareLink{}, ErrShareLinksUnsupported
	}
	location, err := b.shareLocation(location)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	if err := b.shareGate.lock(ctx); err != nil {
		return vfs.ShareLink{}, err
	}
	defer b.shareGate.unlock()

	resource, err := b.shareResource(ctx, location)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	if link, published, err := yandexShareLink(resource); err != nil {
		return vfs.ShareLink{}, err
	} else if published {
		return link, nil
	}
	if err := b.shareMutation(ctx, location, "/resources/publish", "Yandex.Disk publish share link"); err != nil {
		return vfs.ShareLink{}, err
	}
	resource, err = b.shareResource(ctx, location)
	if err != nil {
		return vfs.ShareLink{}, b.rollbackPublishedShare(ctx, location, err)
	}
	link, published, err := yandexShareLink(resource)
	if err != nil {
		return vfs.ShareLink{}, b.rollbackPublishedShare(ctx, location, err)
	}
	if !published {
		err = errors.New("cloudfox: Yandex.Disk published the resource without returning a public URL")
		return vfs.ShareLink{}, b.rollbackPublishedShare(ctx, location, err)
	}
	return link, nil
}

func (b *yandexDiskBackend) RevokeShareLink(ctx context.Context, location string) error {
	location, err := b.shareLocation(location)
	if err != nil {
		return err
	}
	if err := b.shareGate.lock(ctx); err != nil {
		return err
	}
	defer b.shareGate.unlock()
	return b.shareMutation(ctx, location, "/resources/unpublish", "Yandex.Disk revoke share link")
}

var _ BackendShareLinker = (*yandexDiskBackend)(nil)
