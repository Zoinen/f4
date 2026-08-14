package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	drive "google.golang.org/api/drive/v3"

	"github.com/unxed/f4/vfs"
)

const (
	googleShareFileFields       = "id,name,mimeType,webViewLink,resourceKey,shortcutDetails(targetId,targetMimeType,targetResourceKey),capabilities(canShare)"
	googleSharePermissionFields = "id,type,role,view,allowFileDiscovery,permissionDetails(permissionType,inheritedFrom,role,inherited)"
	googleSharePermissionList   = "nextPageToken,permissions(" + googleSharePermissionFields + ")"
)

var googleShareRoles = []vfs.ShareRole{
	vfs.ShareRoleViewer,
	vfs.ShareRoleCommenter,
	vfs.ShareRoleEditor,
}

// googleShareGate serializes permission read/modify/write cycles while still
// allowing a caller canceled in the queue to return immediately.
type googleShareGate struct {
	once sync.Once
	ch   chan struct{}
}

func (gate *googleShareGate) lock(ctx context.Context) error {
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

func (gate *googleShareGate) unlock() { gate.ch <- struct{}{} }

type googleAnyoneShareState struct {
	direct                []*drive.Permission
	inherited             bool
	inheritedRole         vfs.ShareRole
	discoverable          bool
	inheritedDiscoverable bool
	published             bool
	role                  vfs.ShareRole
}

func googleShareRole(role vfs.ShareRole) (string, error) {
	switch role {
	case vfs.ShareRoleViewer:
		return "reader", nil
	case vfs.ShareRoleCommenter:
		return "commenter", nil
	case vfs.ShareRoleEditor:
		return "writer", nil
	default:
		return "", fmt.Errorf("%w: unsupported Google Drive sharing role", os.ErrInvalid)
	}
}

func googleVFSShareRole(role string) (vfs.ShareRole, bool) {
	switch role {
	case "reader":
		return vfs.ShareRoleViewer, true
	case "commenter":
		return vfs.ShareRoleCommenter, true
	case "writer":
		return vfs.ShareRoleEditor, true
	default:
		return 0, false
	}
}

func googleShareRoleRank(role vfs.ShareRole) int {
	switch role {
	case vfs.ShareRoleViewer:
		return 1
	case vfs.ShareRoleCommenter:
		return 2
	case vfs.ShareRoleEditor:
		return 3
	default:
		return 0
	}
}

func googleAllowedShareRoles(state googleAnyoneShareState) []vfs.ShareRole {
	minimum := googleShareRoleRank(state.inheritedRole)
	roles := make([]vfs.ShareRole, 0, len(googleShareRoles))
	for _, role := range googleShareRoles {
		if googleShareRoleRank(role) >= minimum {
			roles = append(roles, role)
		}
	}
	return roles
}

func googleHasDirectDiscoverablePermission(state googleAnyoneShareState) bool {
	for _, permission := range state.direct {
		if permission != nil && permission.AllowFileDiscovery {
			return true
		}
	}
	return false
}

func googlePermissionDirect(permission *drive.Permission) bool {
	if permission == nil {
		return false
	}
	// My Drive historically omitted permissionDetails for direct grants. The
	// permission itself is still directly manageable in that case.
	if len(permission.PermissionDetails) == 0 {
		return true
	}
	for _, detail := range permission.PermissionDetails {
		if detail != nil && !detail.Inherited {
			return true
		}
	}
	return false
}

func googlePermissionInherited(permission *drive.Permission) bool {
	if permission == nil {
		return false
	}
	for _, detail := range permission.PermissionDetails {
		if detail != nil && detail.Inherited {
			return true
		}
	}
	return false
}

func (state *googleAnyoneShareState) add(permission *drive.Permission) error {
	if permission == nil || permission.Type != "anyone" {
		return nil
	}
	// view=metadata is a limited-access-folder metadata grant, not access to
	// the resource contents. A published Workspace view is real public
	// exposure, but Drive does not return its published URL here; record it as
	// separate provider-managed access instead of falsely reporting the item
	// private or making ordinary link-permission reconciliation ambiguous.
	switch permission.View {
	case "":
	case "metadata":
		return nil
	case "published":
		state.published = true
		return nil
	default:
		return fmt.Errorf("cloudfox: Google Drive returned unsupported public permission view %q", permission.View)
	}
	role, ok := googleVFSShareRole(permission.Role)
	if !ok {
		for _, detail := range permission.PermissionDetails {
			if detail == nil {
				continue
			}
			if detailRole, detailOK := googleVFSShareRole(detail.Role); detailOK && googleShareRoleRank(detailRole) > googleShareRoleRank(role) {
				role = detailRole
				ok = true
			}
		}
	}
	if !ok {
		return fmt.Errorf("cloudfox: Google Drive returned unsupported link role %q", permission.Role)
	}
	for _, detail := range permission.PermissionDetails {
		if detail == nil {
			continue
		}
		if detailRole, detailOK := googleVFSShareRole(detail.Role); detailOK && googleShareRoleRank(detailRole) > googleShareRoleRank(role) {
			role = detailRole
		}
		if detail.Inherited {
			state.inherited = true
			if detailRole, detailOK := googleVFSShareRole(detail.Role); detailOK && googleShareRoleRank(detailRole) > googleShareRoleRank(state.inheritedRole) {
				state.inheritedRole = detailRole
			}
		}
	}
	if googleShareRoleRank(role) > googleShareRoleRank(state.role) {
		state.role = role
	}
	state.inherited = state.inherited || googlePermissionInherited(permission)
	if state.inherited && state.inheritedRole == 0 && !googlePermissionDirect(permission) {
		state.inheritedRole = role
	}
	state.discoverable = state.discoverable || permission.AllowFileDiscovery
	if permission.AllowFileDiscovery && googlePermissionInherited(permission) {
		state.inheritedDiscoverable = true
	}
	if googlePermissionDirect(permission) {
		state.direct = append(state.direct, permission)
	}
	return nil
}

func (state googleAnyoneShareState) directPermission() *drive.Permission {
	var fallback *drive.Permission
	for _, permission := range state.direct {
		if permission == nil {
			continue
		}
		if fallback == nil {
			fallback = permission
		}
		if !permission.AllowFileDiscovery {
			return permission
		}
	}
	return fallback
}

func (b *googleDriveBackend) getShareFile(ctx context.Context, itemID, resourceKey string) (*drive.File, error) {
	call := b.service.Files.Get(itemID).
		SupportsAllDrives(true).
		Fields(googleShareFileFields).
		Context(ctx)
	if resourceKey != "" {
		call.Header().Set("X-Goog-Drive-Resource-Keys", itemID+"/"+resourceKey)
	}
	file, err := call.Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	if file == nil || file.Id == "" {
		return nil, errors.New("cloudfox: Google Drive returned incomplete sharing metadata")
	}
	// A shortcut's targetResourceKey is already authoritative. Keep carrying it
	// when files.get omits the otherwise equivalent resourceKey field so every
	// subsequent permissions request can address the same protected target.
	if file.ResourceKey == "" {
		file.ResourceKey = resourceKey
	}
	return file, nil
}

func setGoogleShareResourceKey(header http.Header, fileID, resourceKey string) {
	if resourceKey != "" {
		header.Set("X-Goog-Drive-Resource-Keys", fileID+"/"+resourceKey)
	}
}

func (b *googleDriveBackend) shareTarget(ctx context.Context, location string) (*drive.File, error) {
	parsed, err := parseGoogleLocation(location)
	if err != nil {
		return nil, err
	}
	var file *drive.File
	switch parsed.kind {
	case "item":
		file, err = b.getShareFile(ctx, parsed.itemID, parsed.resourceKey)
	case "shortcut":
		// Canonical shortcut locations carry the target ID but not its resource
		// key. Fetch the shortcut itself so restored sessions can reach targets
		// protected by link-share resource keys.
		file, err = b.getShareFile(ctx, parsed.itemID, parsed.resourceKey)
	case "new":
		file, _, err = b.resolveNew(ctx, parsed)
		if err == nil {
			file, err = b.getShareFile(ctx, file.Id, "")
		}
	default:
		return nil, ErrShareLinksUnsupported
	}
	if err != nil {
		return nil, err
	}
	if file.MimeType == googleShortcutMime && file.ShortcutDetails != nil && file.ShortcutDetails.TargetId != "" {
		return b.getShareFile(ctx, file.ShortcutDetails.TargetId, file.ShortcutDetails.TargetResourceKey)
	}
	return file, nil
}

func (b *googleDriveBackend) anyoneShareState(ctx context.Context, fileID, resourceKey string) (googleAnyoneShareState, error) {
	var state googleAnyoneShareState
	seen := make(map[string]struct{})
	pageToken := ""
	for {
		call := b.service.Permissions.List(fileID).
			PageSize(100).
			SupportsAllDrives(true).
			Fields(googleSharePermissionList).
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		setGoogleShareResourceKey(call.Header(), fileID, resourceKey)
		page, err := call.Do()
		if err != nil {
			return googleAnyoneShareState{}, mapGoogleError(err)
		}
		for _, permission := range page.Permissions {
			if permission == nil || permission.Type != "anyone" {
				continue
			}
			if permission.Id != "" {
				if _, duplicate := seen[permission.Id]; duplicate {
					continue
				}
				seen[permission.Id] = struct{}{}
			}
			if err := state.add(permission); err != nil {
				return googleAnyoneShareState{}, err
			}
		}
		if page.NextPageToken == "" {
			return state, nil
		}
		pageToken = page.NextPageToken
	}
}

func googleShareNotice(state googleAnyoneShareState, canShare bool) string {
	parts := []string{"Google Drive links do not expire; access is controlled by Drive permissions."}
	if state.published {
		parts = append(parts, "A Google Workspace published view also exposes this item; manage that publication in its Google editor.")
	}
	if state.inheritedDiscoverable {
		parts = append(parts, "An inherited public permission allows this item to be discovered in search; change that permission on its parent.")
	} else if state.discoverable {
		parts = append(parts, "The current public permission also allows the item to be discovered in search; creating a link will change it to link-only access.")
	}
	if state.inherited {
		parts = append(parts, "Current public access is inherited from a parent and cannot be reduced below that inherited level or fully revoked on this item.")
	}
	if !canShare {
		parts = append(parts, "Your Google Drive permissions do not allow changing link access for this item.")
	}
	return strings.Join(parts, " ")
}

func googleShareLink(file *drive.File, state googleAnyoneShareState, canShare bool) *vfs.ShareLink {
	if state.role == 0 || file == nil || file.WebViewLink == "" {
		return nil
	}
	revocable := canShare && len(state.direct) != 0 && !state.inherited
	return &vfs.ShareLink{URL: file.WebViewLink, Role: state.role, Revocable: revocable}
}

func googleCanShare(file *drive.File) bool {
	return file != nil && file.Capabilities != nil && file.Capabilities.CanShare
}

func (b *googleDriveBackend) shareLinkInfoLocked(ctx context.Context, location string) (vfs.ShareLinkInfo, *drive.File, googleAnyoneShareState, error) {
	file, err := b.shareTarget(ctx, location)
	if err != nil {
		return vfs.ShareLinkInfo{}, nil, googleAnyoneShareState{}, err
	}
	if file.WebViewLink == "" {
		return vfs.ShareLinkInfo{}, nil, googleAnyoneShareState{}, errors.New("cloudfox: Google Drive did not provide a browser link for this item")
	}
	if err := validateHTTPSShareURL(file.WebViewLink); err != nil {
		return vfs.ShareLinkInfo{}, nil, googleAnyoneShareState{}, err
	}
	state, err := b.anyoneShareState(ctx, file.Id, file.ResourceKey)
	if err != nil {
		return vfs.ShareLinkInfo{}, nil, googleAnyoneShareState{}, err
	}
	canShare := googleCanShare(file)
	roles := googleAllowedShareRoles(state)
	// An inherited Editor grant is already the strongest public-link role.
	// No direct permission on the child can reduce or meaningfully increase it.
	canCreate := canShare && (googleShareRoleRank(state.inheritedRole) < googleShareRoleRank(vfs.ShareRoleEditor) || googleHasDirectDiscoverablePermission(state))
	link := googleShareLink(file, state, canShare)
	return vfs.ShareLinkInfo{
		Provider:                     "Google Drive",
		ItemName:                     file.Name,
		Roles:                        roles,
		ExpirationOptions:            []time.Duration{0},
		CanCreate:                    canCreate,
		CanRevoke:                    link != nil && link.Revocable,
		UnmanagedPublicAccess:        state.published,
		LinkInherited:                state.inherited,
		LinkDiscoverable:             state.discoverable,
		LinkDiscoverabilityInherited: state.inheritedDiscoverable,
		Link:                         link,
		Notice:                       googleShareNotice(state, canShare),
	}, file, state, nil
}

func (b *googleDriveBackend) ShareLinkInfo(ctx context.Context, location string) (vfs.ShareLinkInfo, error) {
	if err := b.shareGate.lock(ctx); err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	defer b.shareGate.unlock()
	info, _, _, err := b.shareLinkInfoLocked(ctx, location)
	return info, err
}

func (b *googleDriveBackend) CreateShareLink(ctx context.Context, location string, request vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	role, err := googleShareRole(request.Role)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	if request.ExpiresIn != 0 {
		return vfs.ShareLink{}, fmt.Errorf("%w: Google Drive public links cannot expire", os.ErrInvalid)
	}

	if err := b.shareGate.lock(ctx); err != nil {
		return vfs.ShareLink{}, err
	}
	defer b.shareGate.unlock()
	info, file, state, err := b.shareLinkInfoLocked(ctx, location)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	if !info.CanCreate {
		return vfs.ShareLink{}, os.ErrPermission
	}
	requestedRank := googleShareRoleRank(request.Role)
	inheritedRank := googleShareRoleRank(state.inheritedRole)
	if len(state.direct) > 1 {
		return vfs.ShareLink{}, errors.New("cloudfox: Google Drive returned multiple direct public permissions; revoke them before creating a new link")
	}
	direct := state.directPermission()
	if state.inherited && direct == nil && requestedRank <= inheritedRank {
		if requestedRank < inheritedRank {
			return vfs.ShareLink{}, fmt.Errorf("%w: Google Drive link access is inherited from a parent and cannot be reduced on this item", os.ErrPermission)
		}
		return vfs.ShareLink{URL: file.WebViewLink, Role: state.inheritedRole, Revocable: false}, nil
	}
	if state.inherited && direct != nil && requestedRank < inheritedRank {
		return vfs.ShareLink{}, fmt.Errorf("%w: Google Drive link access is inherited from a parent and cannot be reduced on this item", os.ErrPermission)
	}

	permission := &drive.Permission{
		Role:               role,
		AllowFileDiscovery: false,
		ForceSendFields:    []string{"AllowFileDiscovery"},
	}
	if direct == nil {
		permission.Type = "anyone"
		call := b.service.Permissions.Create(file.Id, permission).
			SupportsAllDrives(true).
			Fields(googleSharePermissionFields).
			Context(ctx)
		setGoogleShareResourceKey(call.Header(), file.Id, file.ResourceKey)
		_, err = call.Do()
		err = googleMutationError("create share link", err)
	} else {
		if direct.Id == "" {
			return vfs.ShareLink{}, errors.New("cloudfox: Google Drive returned a sharing permission without an id")
		}
		call := b.service.Permissions.Update(file.Id, direct.Id, permission).
			SupportsAllDrives(true).
			Fields(googleSharePermissionFields).
			Context(ctx)
		setGoogleShareResourceKey(call.Header(), file.Id, file.ResourceKey)
		_, err = call.Do()
		err = googleMutationError("update share link", err)
	}
	if err != nil {
		return vfs.ShareLink{}, err
	}
	effectiveRole := request.Role
	if state.inherited && inheritedRank > requestedRank {
		effectiveRole = state.inheritedRole
	}
	return vfs.ShareLink{URL: file.WebViewLink, Role: effectiveRole, Revocable: !state.inherited}, nil
}

func (b *googleDriveBackend) RevokeShareLink(ctx context.Context, location string) error {
	if err := b.shareGate.lock(ctx); err != nil {
		return err
	}
	defer b.shareGate.unlock()
	info, file, state, err := b.shareLinkInfoLocked(ctx, location)
	if err != nil {
		return err
	}
	if info.Link == nil {
		if state.published {
			return fmt.Errorf("%w: Google Workspace published views must be managed in the Google editor", ErrShareLinksUnsupported)
		}
		return nil
	}
	if !info.CanCreate {
		return os.ErrPermission
	}
	if state.inherited {
		return fmt.Errorf("%w: Google Drive link access is inherited from a parent and must be revoked on that parent", os.ErrPermission)
	}
	if len(state.direct) == 0 {
		return nil
	}
	deleted := 0
	for _, permission := range state.direct {
		if permission == nil || permission.Id == "" {
			if deleted != 0 {
				return &vfs.UnknownOperationStateError{Operation: "Google Drive revoke share link", Err: errors.New("sharing permission did not include an id")}
			}
			return errors.New("cloudfox: Google Drive returned a sharing permission without an id")
		}
		call := b.service.Permissions.Delete(file.Id, permission.Id).
			SupportsAllDrives(true).
			Context(ctx)
		setGoogleShareResourceKey(call.Header(), file.Id, file.ResourceKey)
		err := call.Do()
		if err != nil {
			mapped := googleMutationError("revoke share link", err)
			// DELETE 404 is ambiguous: another client may have removed the
			// permission, but Google can also mask an inaccessible file as not
			// found while its public ACL remains active. Force an authoritative
			// follow-up read through the common unknown-state reconciliation.
			if errors.Is(mapped, os.ErrNotExist) {
				return &vfs.UnknownOperationStateError{Operation: "Google Drive revoke share link", Err: mapped}
			}
			if deleted != 0 {
				return &vfs.UnknownOperationStateError{Operation: "Google Drive revoke share link", Err: mapped}
			}
			return mapped
		}
		deleted++
	}
	return nil
}

var _ BackendShareLinker = (*googleDriveBackend)(nil)
