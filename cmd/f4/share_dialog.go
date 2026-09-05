package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func actionShareLink(pf *PanelsFrame) {
	if pf == nil {
		return
	}
	panel := pf.getActivePanel()
	if panel == nil || panel.vfs == nil {
		return
	}
	provider, ok := panel.vfs.(vfs.ShareLinkProvider)
	if !ok {
		return
	}
	names := panel.GetSelectedNames()
	if len(names) != 1 {
		vtui.ShowMessageOn(pf, Msg("Share.Title"), Msg("Share.SelectOne"), []string{Msg("vtui.Ok")})
		return
	}
	path := panel.vfs.Join(panel.vfs.GetPath(), names[0])
	var info vfs.ShareLinkInfo
	pf.RunProgressTask(Msg("Share.LoadTitle"), Msg("Share.Loading"), false, func(ctx context.Context, _ func(string, int)) error {
		var err error
		info, err = provider.ShareLinkInfo(ctx, path)
		return err
	}, func(err error) {
		if err != nil {
			if errors.Is(err, context.Canceled) && !errors.Is(err, vfs.ErrOperationStateUnknown) {
				return
			}
			showShareErrorOn(pf, err)
			return
		}
		if err := validateShareLinkInfo(info); err != nil {
			showShareErrorOn(pf, err)
			return
		}
		showShareLinkDialog(pf, provider, path, info)
	})
}

type shareLinkDialog struct {
	app      *PanelsFrame
	provider vfs.ShareLinkProvider
	path     string
	info     vfs.ShareLinkInfo
	dialog   *vtui.Window

	role             *vtui.ComboBox
	expiration       *vtui.ComboBox
	link             *vtui.Text
	status           *vtui.Text
	noticeLines      []*vtui.Text
	create           *vtui.Button
	copy             *vtui.Button
	revoke           *vtui.Button
	closed           bool
	busy             bool
	stateUnknown     bool
	unexpectedAccess bool
	expiryTimer      *time.Timer
	expiryGeneration uint64
	clipboardStateMu sync.Mutex
	clipboardWriteMu sync.Mutex
	clipboardGen     uint64
	setClipboard     func(string)
}

func showShareLinkDialog(app *PanelsFrame, provider vfs.ShareLinkProvider, path string, info vfs.ShareLinkInfo) *shareLinkDialog {
	d := &shareLinkDialog{app: app, provider: provider, path: path, info: info}
	d.dialog = vtui.NewCenteredDialog(78, 19, Msg("Share.Title"))
	d.dialog.ShowClose = true

	labelX := d.dialog.X1 + 2
	fieldX := d.dialog.X1 + 19
	fieldWidth := 56
	row := d.dialog.Y1 + 2
	addValue := func(label, value string) {
		d.dialog.AddItem(vtui.NewText(labelX, row, label, vtui.Palette[vtui.ColDialogText]))
		d.dialog.AddItem(vtui.NewText(fieldX, row, vtui.TruncateMiddle(value, fieldWidth), vtui.Palette[vtui.ColDialogText]))
		row++
	}
	addValue(Msg("Share.Provider"), info.Provider)
	itemName := info.ItemName
	if itemName == "" {
		itemName = path
	}
	addValue(Msg("Share.Item"), itemName)

	roleLabels := make([]string, len(info.Roles))
	for i, role := range info.Roles {
		roleLabels[i] = shareRoleLabel(role)
	}
	if len(roleLabels) == 0 {
		roleLabels = []string{Msg("Share.NotAvailable")}
	}
	roleIndex := 0
	if info.Link != nil {
		for i, role := range info.Roles {
			if role == info.Link.Role {
				roleIndex = i
				break
			}
		}
	}
	d.dialog.AddItem(vtui.NewText(labelX, row, Msg("Share.Access"), vtui.Palette[vtui.ColDialogText]))
	d.role = vtui.NewComboBox(fieldX, row, fieldWidth, roleLabels)
	d.role.DropdownOnly = true
	d.role.Menu.SetSelectPos(roleIndex)
	d.role.Edit.SetText(roleLabels[roleIndex])
	d.role.SetDisabled(len(info.Roles) <= 1 || !d.canCreate())
	d.dialog.AddItem(d.role)
	row++

	expirationLabels := make([]string, len(info.ExpirationOptions))
	for i, duration := range info.ExpirationOptions {
		expirationLabels[i] = shareExpirationLabel(duration)
	}
	if len(expirationLabels) == 0 {
		if info.Link != nil && info.Link.Role == vfs.ShareRoleServerControlled {
			expirationLabels = []string{Msg("Share.Expiration.ServerControlled")}
		} else {
			expirationLabels = []string{Msg("Share.Expiration.Never")}
		}
	}
	expirationIndex := 0
	for i, duration := range info.ExpirationOptions {
		if duration == info.DefaultExpiration {
			expirationIndex = i
			break
		}
	}
	d.dialog.AddItem(vtui.NewText(labelX, row, Msg("Share.Expiration"), vtui.Palette[vtui.ColDialogText]))
	d.expiration = vtui.NewComboBox(fieldX, row, fieldWidth, expirationLabels)
	d.expiration.DropdownOnly = true
	d.expiration.Menu.SetSelectPos(expirationIndex)
	d.expiration.Edit.SetText(expirationLabels[expirationIndex])
	d.expiration.SetDisabled(len(info.ExpirationOptions) <= 1 || !d.canCreate())
	d.dialog.AddItem(d.expiration)
	row += 2

	for _, line := range formattedShareNotice(info) {
		text := vtui.NewText(labelX, row, line, vtui.Palette[vtui.ColDialogText])
		d.noticeLines = append(d.noticeLines, text)
		d.dialog.AddItem(text)
		row++
	}
	for row < d.dialog.Y2-5 {
		row++
	}

	d.dialog.AddItem(vtui.NewText(labelX, d.dialog.Y2-5, Msg("Share.Link"), vtui.Palette[vtui.ColDialogText]))
	d.link = vtui.NewText(labelX, d.dialog.Y2-4, "", vtui.Palette[vtui.ColDialogText])
	d.link.SetPosition(labelX, d.dialog.Y2-4, d.dialog.X2-2, d.dialog.Y2-4)
	d.dialog.AddItem(d.link)
	d.status = vtui.NewText(labelX, d.dialog.Y2-3, "", vtui.Palette[vtui.ColDialogText])
	d.status.SetPosition(labelX, d.dialog.Y2-3, d.dialog.X2-2, d.dialog.Y2-3)
	d.dialog.AddItem(d.status)

	d.create = vtui.NewButton(0, 0, Msg("Share.CreateCopy"))
	d.copy = vtui.NewButton(0, 0, Msg("Share.Copy"))
	d.revoke = vtui.NewButton(0, 0, Msg("Share.Revoke"))
	closeButton := vtui.NewButton(0, 0, Msg("Share.Close"))
	d.dialog.AddItem(d.create)
	d.dialog.AddItem(d.copy)
	d.dialog.AddItem(d.revoke)
	d.dialog.AddItem(closeButton)
	buttons := vtui.NewHBoxLayout(d.dialog.X1+2, d.dialog.Y2-1, 74, 1)
	buttons.Spacing = 1
	buttons.Add(d.create, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(d.copy, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(d.revoke, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(closeButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Apply()

	d.create.OnClick = d.onCreate
	d.copy.OnClick = d.onCopy
	d.revoke.OnClick = d.onRevoke
	closeButton.OnClick = d.dialog.Close
	d.dialog.OnResult = func(int) {
		d.closed = true
		if d.expiryTimer != nil {
			d.expiryTimer.Stop()
		}
	}
	d.refresh("")
	if info.Link != nil {
		d.copy.IsDefault = true
		d.dialog.SetFocusedItem(d.copy)
	} else if d.canCreate() {
		d.create.IsDefault = true
		d.dialog.SetFocusedItem(d.create)
	} else {
		closeButton.IsDefault = true
		d.dialog.SetFocusedItem(closeButton)
	}
	if app != nil {
		vtui.FrameManager.PushToFrameScreen(app, d.dialog)
	} else {
		vtui.FrameManager.Push(d.dialog)
	}
	return d
}

func (d *shareLinkDialog) request() (vfs.ShareLinkRequest, bool) {
	roleIndex := d.role.Menu.SelectPos
	if roleIndex < 0 || roleIndex >= len(d.info.Roles) {
		return vfs.ShareLinkRequest{}, false
	}
	request := vfs.ShareLinkRequest{Role: d.info.Roles[roleIndex]}
	if len(d.info.ExpirationOptions) != 0 {
		expirationIndex := d.expiration.Menu.SelectPos
		if expirationIndex < 0 || expirationIndex >= len(d.info.ExpirationOptions) {
			return vfs.ShareLinkRequest{}, false
		}
		request.ExpiresIn = d.info.ExpirationOptions[expirationIndex]
	}
	return request, true
}

func (d *shareLinkDialog) canCreate() bool {
	return d != nil && d.info.CanCreate && len(d.info.Roles) != 0
}

func validShareRole(role vfs.ShareRole) bool {
	return role >= vfs.ShareRoleViewer && role <= vfs.ShareRoleServerControlled
}

func validShareDisplayText(value string, maximum int) bool {
	return len(value) <= maximum && strings.IndexFunc(value, func(r rune) bool {
		joinControl := unicode.Properties["Join_Control"]
		allowedJoinControl := joinControl != nil && unicode.Is(joinControl, r)
		return unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp) || (unicode.Is(unicode.Cf, r) && !allowedJoinControl)
	}) < 0
}

func validateShareLinkInfo(info vfs.ShareLinkInfo) error {
	if !validShareDisplayText(info.Provider, 256) || !validShareDisplayText(info.ItemName, 4096) ||
		!validShareDisplayText(info.Notice, 8192) || len(info.Roles) > 16 || len(info.ExpirationOptions) > 32 {
		return errors.New("share provider returned invalid metadata")
	}
	if shareErrorURLPattern.MatchString(info.Notice) || shareErrorSecretPattern.MatchString(info.Notice) {
		return errors.New("share provider returned unsafe explanatory text")
	}
	for _, role := range info.Roles {
		if !validShareRole(role) {
			return errors.New("share provider returned an invalid access role")
		}
	}
	for _, expiration := range info.ExpirationOptions {
		if expiration < 0 {
			return errors.New("share provider returned an invalid expiration")
		}
	}
	if info.DefaultExpiration != 0 {
		found := false
		for _, expiration := range info.ExpirationOptions {
			found = found || expiration == info.DefaultExpiration
		}
		if !found {
			return errors.New("share provider returned an unsupported default expiration")
		}
	}
	if info.Link != nil {
		if !validShareRole(info.Link.Role) {
			return errors.New("share provider returned an invalid link role")
		}
		if err := vfs.ValidateShareURL(info.Link.URL); err != nil {
			return err
		}
		roleFound := false
		for _, role := range info.Roles {
			roleFound = roleFound || role == info.Link.Role
		}
		if !roleFound {
			return errors.New("share provider did not expose the active link role")
		}
		if info.Link.ExpiresAtIsMaximum && info.Link.ExpiresAt.IsZero() {
			return errors.New("share provider returned an invalid link expiration")
		}
		if info.CanRevoke != info.Link.Revocable {
			return errors.New("share provider returned inconsistent revocation capabilities")
		}
	} else if info.CanRevoke {
		return errors.New("share provider can revoke a missing link")
	}
	return nil
}

func (d *shareLinkDialog) onCreate() {
	if d.busy || d.stateUnknown || d.unexpectedAccess || !d.canCreate() {
		return
	}
	request, ok := d.request()
	if !ok {
		return
	}
	d.setBusy(true)
	var link vfs.ShareLink
	issuedAfter := time.Now()
	d.app.RunProgressTask(Msg("Share.CreateTitle"), Msg("Share.Creating"), false, func(ctx context.Context, _ func(string, int)) error {
		var err error
		link, err = d.provider.CreateShareLink(ctx, d.path, request)
		return err
	}, func(err error) {
		if err == nil {
			if validationErr := vfs.ValidateCreatedShareLink(link, request, issuedAfter, time.Now()); validationErr != nil {
				err = &vfs.UnknownOperationStateError{Operation: "create share link", Err: validationErr}
			}
		}
		if errors.Is(err, vfs.ErrOperationStateUnknown) {
			d.reconcileCreate(err, request, issuedAfter)
			return
		}
		d.setBusy(false)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				if !d.closed {
					d.refresh(Msg("Share.Canceled"))
				}
				return
			}
			if !d.closed {
				d.refresh(Msg("Share.CreateFailed"))
			}
			showShareErrorOn(d.reconciliationAnchor(), err)
			return
		}
		d.stateUnknown = false
		d.unexpectedAccess = false
		d.info.Link = &link
		d.info.CanRevoke = link.Revocable
		if !d.info.LinkDiscoverabilityInherited {
			d.info.LinkDiscoverable = false
		}
		d.copyLinkToClipboard(link.URL)
		if d.closed {
			showShareMessageOn(d.reconciliationAnchor(), Msg("Share.StateTitle"), Msg("Share.CreatedCopiedAfterClose"))
			return
		}
		d.selectLinkRole()
		d.refresh(Msg("Share.Copied"))
	})
}

func (d *shareLinkDialog) onCopy() {
	if d.busy || d.stateUnknown || d.unexpectedAccess || !shareLinkCopyableAt(d.info.Link, time.Now()) {
		return
	}
	link := d.info.Link.URL
	d.refresh(Msg("Share.Copied"))
	d.copyLinkToClipboard(link)
}

func (d *shareLinkDialog) onRevoke() {
	if d.busy || d.stateUnknown || d.info.Link == nil || !d.info.CanRevoke {
		return
	}
	confirm := vtui.ShowMessageOn(d.dialog, Msg("Share.RevokeTitle"), Msg("Share.RevokeConfirm"), []string{Msg("Share.Revoke"), Msg("vtui.Cancel")})
	confirm.OnResult = func(code int) {
		if code != 0 || d.closed {
			return
		}
		d.setBusy(true)
		d.app.RunProgressTask(Msg("Share.RevokeTitle"), Msg("Share.Revoking"), false, func(ctx context.Context, _ func(string, int)) error {
			return d.provider.RevokeShareLink(ctx, d.path)
		}, func(err error) {
			if errors.Is(err, vfs.ErrOperationStateUnknown) {
				d.reconcileRevoke(err)
				return
			}
			d.setBusy(false)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					if !d.closed {
						d.refresh(Msg("Share.Canceled"))
					}
					return
				}
				if !d.closed {
					d.refresh(Msg("Share.RevokeFailed"))
				}
				showShareErrorOn(d.reconciliationAnchor(), err)
				return
			}
			d.stateUnknown = false
			d.unexpectedAccess = false
			d.info.Link = nil
			d.info.CanRevoke = false
			d.info.LinkInherited = false
			d.info.LinkDiscoverable = false
			d.info.LinkDiscoverabilityInherited = false
			if d.closed {
				showShareMessageOn(d.reconciliationAnchor(), Msg("Share.StateTitle"), Msg("Share.RevokedAfterClose"))
				return
			}
			d.refresh(Msg("Share.Revoked"))
		})
	}
}

func (d *shareLinkDialog) selectLinkRole() {
	if d.info.Link == nil || d.role == nil {
		return
	}
	for i, role := range d.info.Roles {
		if role == d.info.Link.Role && i < len(d.role.Menu.Items) {
			d.role.Menu.SetSelectPos(i)
			d.role.Edit.SetText(shareRoleLabel(role))
			return
		}
	}
}

func (d *shareLinkDialog) reconciliationAnchor() vtui.Frame {
	if d != nil && !d.closed && d.dialog != nil {
		return d.dialog
	}
	if d != nil && d.app != nil {
		return d.app
	}
	return nil
}

func (d *shareLinkDialog) reconcileCreate(original error, request vfs.ShareLinkRequest, issuedAfter time.Time) {
	if d.app == nil {
		d.stateUnknown = true
		d.setBusy(false)
		showShareErrorOn(d.reconciliationAnchor(), original)
		return
	}
	var info vfs.ShareLinkInfo
	d.app.RunProgressTask(Msg("Share.ReconcileTitle"), Msg("Share.Reconciling"), false, func(ctx context.Context, _ func(string, int)) error {
		var err error
		info, err = d.provider.ShareLinkInfo(ctx, d.path)
		if err == nil {
			err = validateShareLinkInfo(info)
		}
		return err
	}, func(reconcileErr error) {
		if reconcileErr != nil {
			d.stateUnknown = true
			d.unexpectedAccess = false
			d.setBusy(false)
			if !d.closed {
				d.refresh(Msg("Share.StateUnknown"))
			}
			showShareErrorOn(d.reconciliationAnchor(), &vfs.UnknownOperationStateError{Operation: "reconcile share link", Err: reconcileErr})
			return
		}
		d.stateUnknown = false
		if info.Link != nil {
			link := *info.Link
			d.applyAuthoritativeInfo(info)
			if err := vfs.ValidateCreatedShareLink(link, request, issuedAfter, time.Now()); err != nil {
				d.unexpectedAccess = true
				d.setBusy(false)
				if !d.closed {
					d.selectLinkRole()
					d.refresh(Msg("Share.UnexpectedAccess"))
				}
				showShareMessageOn(d.reconciliationAnchor(), Msg("Share.StateTitle"), Msg("Share.UnexpectedAccess"))
				return
			}
			d.unexpectedAccess = false
			d.setBusy(false)
			d.copyLinkToClipboard(link.URL)
			if d.closed {
				showShareMessageOn(d.reconciliationAnchor(), Msg("Share.StateTitle"), Msg("Share.ReconciledCopied"))
				return
			}
			d.selectLinkRole()
			d.refresh(Msg("Share.ReconciledCopied"))
			return
		}
		if info.LinksUnenumerable {
			d.stateUnknown = true
			d.unexpectedAccess = false
			d.applyAuthoritativeInfo(info)
			d.setBusy(false)
			if !d.closed {
				d.refresh(Msg("Share.StateUnknown"))
			}
			showShareErrorOn(d.reconciliationAnchor(), &vfs.UnknownOperationStateError{Operation: "reconcile share link"})
			return
		}
		// A fresh authoritative read proved that no link is active. The original
		// uncertain create did not leave public access behind.
		d.applyAuthoritativeInfo(info)
		d.unexpectedAccess = false
		d.setBusy(false)
		if d.closed {
			showShareMessageOn(d.reconciliationAnchor(), Msg("Share.StateTitle"), Msg("Share.NotCreated"))
			return
		}
		d.refresh(Msg("Share.NotCreated"))
	})
}

func (d *shareLinkDialog) reconcileRevoke(original error) {
	if d.app == nil {
		d.stateUnknown = true
		d.setBusy(false)
		showShareErrorOn(d.reconciliationAnchor(), original)
		return
	}
	var info vfs.ShareLinkInfo
	d.app.RunProgressTask(Msg("Share.ReconcileTitle"), Msg("Share.Reconciling"), false, func(ctx context.Context, _ func(string, int)) error {
		var err error
		info, err = d.provider.ShareLinkInfo(ctx, d.path)
		if err == nil {
			err = validateShareLinkInfo(info)
		}
		return err
	}, func(reconcileErr error) {
		if reconcileErr != nil {
			d.stateUnknown = true
			d.unexpectedAccess = false
			d.setBusy(false)
			if !d.closed {
				d.refresh(Msg("Share.StateUnknown"))
			}
			showShareErrorOn(d.reconciliationAnchor(), &vfs.UnknownOperationStateError{Operation: "reconcile revoked share link", Err: reconcileErr})
			return
		}
		d.stateUnknown = false
		d.unexpectedAccess = false
		if info.Link == nil {
			d.applyAuthoritativeInfo(info)
			d.setBusy(false)
			if !d.closed {
				d.refresh(Msg("Share.Revoked"))
			} else {
				showShareMessageOn(d.reconciliationAnchor(), Msg("Share.StateTitle"), Msg("Share.RevokedAfterClose"))
			}
			return
		}
		if info.Link != nil {
			d.applyAuthoritativeInfo(info)
			d.setBusy(false)
			if !d.closed {
				d.selectLinkRole()
				d.refresh(Msg("Share.RevokeStillActive"))
			}
			showShareMessageOn(d.reconciliationAnchor(), Msg("Share.StateTitle"), Msg("Share.RevokeStillActive"))
			return
		}
	})
}

func (d *shareLinkDialog) setBusy(busy bool) {
	d.busy = busy
	mutationsBlocked := busy || d.stateUnknown || d.unexpectedAccess
	d.create.SetDisabled(mutationsBlocked || !d.canCreate())
	d.copy.SetDisabled(mutationsBlocked || !shareLinkCopyableAt(d.info.Link, time.Now()))
	// An authoritative but stronger/longer-than-requested result is unsafe to
	// copy or modify again, but Revoke is the remediation and must remain usable.
	d.revoke.SetDisabled(busy || d.stateUnknown || d.info.Link == nil || !d.info.CanRevoke)
	d.role.SetDisabled(mutationsBlocked || len(d.info.Roles) <= 1 || !d.canCreate())
	d.expiration.SetDisabled(mutationsBlocked || len(d.info.ExpirationOptions) <= 1 || !d.canCreate())
}

func (d *shareLinkDialog) refresh(status string) {
	d.refreshNotice()
	if d.info.Link == nil || d.info.Link.URL == "" {
		if d.info.LinksUnenumerable {
			d.link.SetText(Msg("Share.LinkUnenumerable"))
			if status == "" {
				status = Msg("Share.UnenumerableStatus")
			}
		} else if d.info.UnmanagedPublicAccess {
			d.link.SetText(Msg("Share.UnmanagedAccess"))
			if status == "" {
				status = Msg("Share.UnmanagedStatus")
			}
		} else {
			d.link.SetText(Msg("Share.NoLink"))
			if status == "" {
				status = Msg("Share.Ready")
			}
		}
	} else {
		d.link.SetText(vtui.TruncateMiddle(d.info.Link.URL, 72))
		if status == "" {
			if d.info.Link.Role == vfs.ShareRoleServerControlled {
				status = Msg("Share.ServerControlledStatus")
			} else if !shareLinkCopyableAt(d.info.Link, time.Now()) {
				status = Msg("Share.Expired")
			} else if d.info.Link.ExpiresAt.IsZero() {
				status = Msg("Share.Active")
			} else if d.info.Link.ExpiresAtIsMaximum {
				status = fmt.Sprintf(Msg("Share.ActiveNoLaterThan"), d.info.Link.ExpiresAt.Local().Format(time.RFC822))
			} else {
				status = fmt.Sprintf(Msg("Share.ActiveUntil"), d.info.Link.ExpiresAt.Local().Format(time.RFC822))
			}
		}
	}
	d.status.SetText(vtui.TruncateMiddle(status, 72))
	d.setBusy(d.busy)
	d.scheduleExpiryRefresh()
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

func formattedShareNotice(info vfs.ShareLinkInfo) []string {
	lines := vtui.WrapText(shareNotice(info), 72)
	if len(lines) > 3 {
		lines = append(lines[:2], vtui.TruncateMiddle(strings.Join(lines[2:], " "), 72))
	}
	for len(lines) < 3 {
		lines = append(lines, "")
	}
	return lines
}

func (d *shareLinkDialog) refreshNotice() {
	lines := formattedShareNotice(d.info)
	for index, text := range d.noticeLines {
		if index < len(lines) {
			text.SetText(lines[index])
		}
	}
}

func setShareComboItems(combo *vtui.ComboBox, labels []string, selected int) {
	if combo == nil || len(labels) == 0 {
		return
	}
	combo.Menu.Items = make([]vtui.MenuItem, len(labels))
	for i, label := range labels {
		combo.Menu.Items[i] = vtui.MenuItem{Text: label}
	}
	combo.Menu.ItemCount = len(labels)
	if selected < 0 || selected >= len(labels) {
		selected = 0
	}
	combo.Menu.SetSelectPos(selected)
	combo.Edit.SetText(labels[selected])
}

func (d *shareLinkDialog) syncShareSelectors() {
	roleLabels := make([]string, len(d.info.Roles))
	roleIndex := 0
	for i, role := range d.info.Roles {
		roleLabels[i] = shareRoleLabel(role)
		if d.info.Link != nil && role == d.info.Link.Role {
			roleIndex = i
		}
	}
	if len(roleLabels) == 0 {
		roleLabels = []string{Msg("Share.NotAvailable")}
	}
	setShareComboItems(d.role, roleLabels, roleIndex)

	expirationLabels := make([]string, len(d.info.ExpirationOptions))
	expirationIndex := 0
	for i, duration := range d.info.ExpirationOptions {
		expirationLabels[i] = shareExpirationLabel(duration)
		if duration == d.info.DefaultExpiration {
			expirationIndex = i
		}
	}
	if len(expirationLabels) == 0 {
		if d.info.Link != nil && d.info.Link.Role == vfs.ShareRoleServerControlled {
			expirationLabels = []string{Msg("Share.Expiration.ServerControlled")}
		} else {
			expirationLabels = []string{Msg("Share.Expiration.Never")}
		}
	}
	setShareComboItems(d.expiration, expirationLabels, expirationIndex)
}

func (d *shareLinkDialog) applyAuthoritativeInfo(info vfs.ShareLinkInfo) {
	info.Roles = append([]vfs.ShareRole(nil), info.Roles...)
	info.ExpirationOptions = append([]time.Duration(nil), info.ExpirationOptions...)
	if info.Link != nil {
		link := *info.Link
		info.Link = &link
	}
	d.info = info
	d.syncShareSelectors()
}

func (d *shareLinkDialog) copyLinkToClipboard(link string) {
	d.clipboardStateMu.Lock()
	d.clipboardGen++
	generation := d.clipboardGen
	d.clipboardStateMu.Unlock()
	go func() {
		d.clipboardWriteMu.Lock()
		defer d.clipboardWriteMu.Unlock()
		d.clipboardStateMu.Lock()
		current := generation == d.clipboardGen
		d.clipboardStateMu.Unlock()
		if current {
			setClipboard := d.setClipboard
			if setClipboard == nil {
				setClipboard = vtui.SetClipboard
			}
			setClipboard(link)
		}
	}()
}

func shareLinkCopyableAt(link *vfs.ShareLink, now time.Time) bool {
	return link != nil && link.URL != "" && (link.ExpiresAt.IsZero() || now.Before(link.ExpiresAt))
}

func (d *shareLinkDialog) scheduleExpiryRefresh() {
	d.expiryGeneration++
	generation := d.expiryGeneration
	if d.expiryTimer != nil {
		d.expiryTimer.Stop()
		d.expiryTimer = nil
	}
	if d.closed || d.info.Link == nil || d.info.Link.ExpiresAt.IsZero() {
		return
	}
	delay := time.Until(d.info.Link.ExpiresAt)
	if delay <= 0 {
		return
	}
	expiry := d.info.Link.ExpiresAt
	linkURL := d.info.Link.URL
	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	d.expiryTimer = time.AfterFunc(delay, func() {
		if frames == nil {
			return
		}
		frames.PostTask(func() {
			if !d.expiryTimerStillCurrent(generation, linkURL, expiry, time.Now()) {
				return
			}
			d.refresh(Msg("Share.Expired"))
		})
	})
}

func (d *shareLinkDialog) expiryTimerStillCurrent(generation uint64, linkURL string, expiry, now time.Time) bool {
	return d != nil && !d.closed && generation == d.expiryGeneration && d.info.Link != nil &&
		d.info.Link.URL == linkURL && d.info.Link.ExpiresAt.Equal(expiry) && !now.Before(expiry)
}

func shareRoleLabel(role vfs.ShareRole) string {
	switch role {
	case vfs.ShareRoleViewer:
		return Msg("Share.Role.Viewer")
	case vfs.ShareRoleCommenter:
		return Msg("Share.Role.Commenter")
	case vfs.ShareRoleEditor:
		return Msg("Share.Role.Editor")
	case vfs.ShareRoleUploader:
		return Msg("Share.Role.Uploader")
	case vfs.ShareRoleServerControlled:
		return Msg("Share.Role.ServerControlled")
	default:
		return Msg("Share.NotAvailable")
	}
}

func shareExpirationLabel(duration time.Duration) string {
	if duration == 0 {
		return Msg("Share.Expiration.Never")
	}
	if duration%(24*time.Hour) == 0 {
		return fmt.Sprintf(Msg("Share.Expiration.Days"), int(duration/(24*time.Hour)))
	}
	if duration%time.Hour == 0 {
		return fmt.Sprintf(Msg("Share.Expiration.Hours"), int(duration/time.Hour))
	}
	return fmt.Sprintf(Msg("Share.Expiration.Minutes"), int(duration/time.Minute))
}

func shareNotice(info vfs.ShareLinkInfo) string {
	provider := strings.ToLower(info.Provider)
	switch {
	case strings.Contains(provider, "webdav"):
		return Msg("Share.Notice.WebDAV")
	case strings.Contains(provider, "s3"):
		return Msg("Share.Notice.S3")
	case strings.Contains(provider, "yandex"):
		return Msg("Share.Notice.Yandex")
	case strings.Contains(provider, "google"):
		parts := make([]string, 0, 5)
		// Google view=published is public exposure whose URL and lifecycle are
		// controlled by the native Docs/Sheets/Slides editor, not by the Drive
		// anyone permission managed in this dialog. Put the remediation first so
		// the fixed-height notice cannot truncate it behind generic prose.
		if info.UnmanagedPublicAccess {
			parts = append(parts, Msg("Share.Notice.GooglePublished"))
		}
		parts = append(parts, Msg("Share.Notice.Google"))
		if info.LinkDiscoverabilityInherited {
			parts = append(parts, Msg("Share.Notice.GoogleInheritedDiscoverable"))
		} else if info.LinkDiscoverable {
			parts = append(parts, Msg("Share.Notice.GoogleDiscoverable"))
		}
		if info.LinkInherited {
			parts = append(parts, Msg("Share.Notice.GoogleInherited"))
		}
		lowerNotice := strings.ToLower(info.Notice)
		if strings.Contains(lowerNotice, "do not allow") {
			parts = append(parts, Msg("Share.Notice.GoogleReadOnly"))
		}
		return strings.Join(parts, " ")
	case info.Notice != "":
		return info.Notice
	default:
		return Msg("Share.Notice.Public")
	}
}

var (
	shareErrorURLPattern    = regexp.MustCompile(`(?i)https?://[^\s<>"']+`)
	shareErrorSecretPattern = regexp.MustCompile(`(?i)(x-amz-[a-z0-9-]+|x-goog-[a-z0-9-]+|access_token|oauth_token|public_url|public_key|signature|credential|token)=([^&\s]+)`)
)

func safeShareErrorMessage(err error) string {
	message := Msg("Share.ErrorGeneric")
	if err != nil {
		message = strings.TrimSpace(err.Error())
		message = shareErrorURLPattern.ReplaceAllString(message, "[link redacted]")
		message = shareErrorSecretPattern.ReplaceAllString(message, "$1=[redacted]")
		if message == "" {
			message = Msg("Share.ErrorGeneric")
		}
		runes := []rune(message)
		if len(runes) > 4096 {
			message = string(runes[:4093]) + "..."
		}
	}
	return message
}

func showShareErrorOn(anchor vtui.Frame, err error) {
	message := safeShareErrorMessage(err)
	showShareMessageOn(anchor, Msg("Share.ErrorTitle"), message)
}

func showShareMessageOn(anchor vtui.Frame, title, message string) {
	if anchor != nil {
		vtui.ShowMessageOn(anchor, title, message, []string{Msg("vtui.Ok")})
		return
	}
	vtui.ShowMessage(title, message, []string{Msg("vtui.Ok")})
}
