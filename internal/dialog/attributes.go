package dialog

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"os/user"
	"strconv"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"github.com/unxed/vtui"
)

type AttributesTarget struct {
	Path string
	Item vfs.VFSItem
}

func ShowAttributesDialog(refresh func(), v vfs.VFS, path string, item vfs.VFSItem) {
	if litem, err := vfs.Lstat(context.Background(), v, path); err == nil && litem.IsSymlink {
		item = litem
	}
	ShowAttributesDialogForTargets(refresh, v, []AttributesTarget{{Path: path, Item: item}})
}

func ShowAttributesDialogForTargets(refresh func(), v vfs.VFS, targets []AttributesTarget) {
	if v == nil || len(targets) == 0 {
		return
	}
	caps := v.GetCapabilities()
	if !caps.HasUnixPermissions {
		ShowAttributesWindowsForTargets(refresh, v, targets)
	} else {
		ShowAttributesUnixForTargets(refresh, v, targets)
	}
}

func setUnixAttributesForTargets(ctx context.Context, v vfs.VFS, targets []AttributesTarget, edited vfs.VFSItem, preserveUnixMode uint32) error {
	for _, target := range targets {
		item := target.Item
		item.Uid = edited.Uid
		item.Gid = edited.Gid
		item.UnixMode = (item.UnixMode & preserveUnixMode) | (edited.UnixMode &^ preserveUnixMode)
		item.MTime = edited.MTime
		if err := v.SetAttributes(ctx, target.Path, item); err != nil {
			return fmt.Errorf("%s: %w", target.Path, err)
		}
	}
	return nil
}

func ShowSymlinkTargetDialog(refresh func(), v vfs.VFS, path, target string) {
	const width, height = 72, 9
	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("SymlinkEdit.Title"))
	dlg.ShowClose = true

	fileText := vtui.NewText(0, 0,
		fmt.Sprintf(i18n.Msg("SymlinkEdit.File"), vtui.TruncateMiddle(v.Base(path), width-8)),
		vtui.Palette[vtui.ColDialogText])
	editTarget := vtui.NewEdit(0, 0, width-10, target)
	lblTarget := vtui.NewLabel(0, 0, i18n.Msg("SymlinkEdit.Target"), editTarget)
	btnSave := vtui.NewButton(0, 0, i18n.Msg("SymlinkEdit.Save"))
	btnSave.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("SymlinkEdit.Cancel"))

	dlg.AddItem(fileText)
	dlg.AddItem(lblTarget)
	dlg.AddItem(editTarget)
	dlg.AddItem(btnSave)
	dlg.AddItem(btnCancel)

	btnSave.OnClick = func() {
		newTarget := editTarget.GetText()
		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			if err := ReplaceSymlinkTarget(ctx.Context, v, path, newTarget); err != nil {
				ctx.RunOnUI(func() {
					vtui.ShowMessage(i18n.Msg("SymlinkEdit.ErrorTitle"), err.Error(), []string{"&Ok"})
				})
				return
			}
			ctx.RunOnUI(func() {
				dlg.Close()
				if refresh != nil {
					refresh()
				}
			})
		})
	}
	btnCancel.OnClick = func() { dlg.Close() }

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-3)
	vbox.Add(fileText, vtui.Margins{}, vtui.AlignLeft)
	rowTarget := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowTarget.Add(lblTarget, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowTarget.Add(editTarget, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowTarget, vtui.Margins{Top: 1}, vtui.AlignFill)
	rowButtons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowButtons.HorizontalAlign = vtui.AlignCenter
	rowButtons.Spacing = 2
	rowButtons.Add(btnSave, vtui.Margins{}, vtui.AlignTop)
	rowButtons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(rowButtons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()
	dlg.SetFocusedItem(editTarget)
	vtui.FrameManager.Push(dlg)
}

// ReplaceSymlinkTarget changes the link itself, never the object it points at.
// The new link is created only after the old one has been removed because the
// optional VFS API does not promise replace semantics. If creation fails, put
// the original link back before returning the error so a failed edit cannot
// silently delete the user's link.
func ReplaceSymlinkTarget(ctx context.Context, v vfs.VFS, path, newTarget string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if newTarget == "" {
		return errors.New("symlink target cannot be empty")
	}
	symVFS, ok := v.(vfs.SymlinkVFS)
	if !ok {
		return errors.New("VFS does not support symbolic links")
	}
	oldTarget, err := symVFS.Readlink(ctx, path)
	if err != nil {
		return fmt.Errorf("read symlink %q: %w", path, err)
	}
	if oldTarget == newTarget {
		return nil
	}
	if err := v.Remove(ctx, path); err != nil {
		return fmt.Errorf("remove symlink %q: %w", path, err)
	}
	createErr := symVFS.Symlink(ctx, newTarget, path)
	if createErr == nil {
		return nil
	}
	if restoreErr := symVFS.Symlink(ctx, oldTarget, path); restoreErr != nil {
		return fmt.Errorf("create symlink %q: %w; restore original target %q: %v", path, createErr, oldTarget, restoreErr)
	}
	return fmt.Errorf("create symlink %q: %w (original target restored)", path, createErr)
}

func setWindowsAttributesForTargets(ctx context.Context, v vfs.VFS, targets []AttributesTarget, edited vfs.VFSItem, preserveWinAttrs uint32) error {
	const editableWinAttrs = uint32(1 | 2 | 4 | 32)
	for _, target := range targets {
		item := target.Item
		item.MTime = edited.MTime
		if preserveWinAttrs&1 == 0 {
			item.UnixMode = edited.UnixMode
		}
		// The dialog edits only the four ordinary Windows flags. Keep
		// directory/reparse/compression and other provider-specific flags from
		// each target instead of copying those of the first selected object.
		editable := editableWinAttrs &^ preserveWinAttrs
		item.WinAttrs = (item.WinAttrs &^ editable) | (edited.WinAttrs & editable)
		if err := v.SetAttributes(ctx, target.Path, item); err != nil {
			return fmt.Errorf("%s: %w", target.Path, err)
		}
	}
	return nil
}

func ShowAttributesUnix(refresh func(), v vfs.VFS, path string, item vfs.VFSItem) {
	ShowAttributesUnixForTargets(refresh, v, []AttributesTarget{{Path: path, Item: item}})
}

func ShowAttributesUnixForTargets(refresh func(), v vfs.VFS, targets []AttributesTarget) {
	path := targets[0].Path
	item := targets[0].Item
	width, height := 70, 24
	if item.IsSymlink && len(targets) == 1 {
		height = 26
	}

	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("Attributes.Title"))
	dlg.ShowClose = true

	x, y := dlg.X1, dlg.Y1
	const timeFormat = "02.01.2006 15:04:05"

	// Основной контейнер
	mainVBox := vtui.NewVBoxLayout(x+3, y+2, width-6, height-4)

	// Header
	info := fmt.Sprintf("Change file attributes for:\n%s", vtui.TruncateMiddle(v.Base(path), 60))
	lines := vtui.WrapText(info, 60)
	for _, l := range lines {
		t := vtui.NewText(0, 0, l, vtui.Palette[vtui.ColDialogText])
		dlg.AddItem(t)
		mainVBox.Add(t, vtui.Margins{}, vtui.AlignCenter)
	}

	var editTarget *vtui.Edit
	if item.IsSymlink && len(targets) == 1 {
		targetVal, _ := vfs.Readlink(context.Background(), v, path)
		editTarget = vtui.NewEdit(0, 0, 35, targetVal)
		lblTarget := vtui.NewLabel(0, 0, PadLabel(i18n.Msg("Attributes.Target")), editTarget)
		rowTarget := vtui.NewHBoxLayout(0, 0, 66, 1)
		rowTarget.Add(lblTarget, vtui.Margins{Left: 2, Right: 1}, vtui.AlignLeft)
		rowTarget.Add(editTarget, vtui.Margins{}, vtui.AlignFill)
		dlg.AddItem(lblTarget)
		dlg.AddItem(editTarget)
		mainVBox.Add(rowTarget, vtui.Margins{Top: 1}, vtui.AlignFill)
	}

	// Ownership Group
	gbOwnership := vtui.NewGroupBox(0, 0, 66, 4, " "+i18n.Msg("Attributes.Ownership")+" ")
	dlg.AddItem(gbOwnership)
	mainVBox.Add(gbOwnership, vtui.Margins{Top: 1}, vtui.AlignFill)

	// Permissions Group
	// Permissions Group
	gbPerms := vtui.NewGroupBox(0, 0, 66, 7, " "+i18n.Msg("Attributes.Permissions")+" ")
	dlg.AddItem(gbPerms)
	mainVBox.Add(gbPerms, vtui.Margins{Top: 0}, vtui.AlignFill)

	// Time Row
	editMTime := vtui.NewEdit(0, 0, 20, item.MTime.Format(timeFormat))
	lblTime := vtui.NewLabel(0, 0, PadLabel(i18n.Msg("Attributes.MTime")), editMTime)
	rowTime := vtui.NewHBoxLayout(0, 0, 66, 1)
	rowTime.Add(lblTime, vtui.Margins{Left: 2, Right: 1}, vtui.AlignLeft)
	rowTime.Add(editMTime, vtui.Margins{}, vtui.AlignLeft)
	dlg.AddItem(lblTime)
	dlg.AddItem(editMTime)
	mainVBox.Add(rowTime, vtui.Margins{Top: 0}, vtui.AlignFill)

	// Buttons
	btnSet := vtui.NewButton(0, 0, i18n.Msg("Attributes.BtnSet"))
	btnSet.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	rowBtns := vtui.NewHBoxLayout(0, 0, 66, 1)
	rowBtns.HorizontalAlign = vtui.AlignCenter
	rowBtns.Spacing = 2
	rowBtns.Add(btnSet, vtui.Margins{}, vtui.AlignTop)
	rowBtns.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	dlg.AddItem(btnSet)
	dlg.AddItem(btnCancel)
	mainVBox.Add(rowBtns, vtui.Margins{Top: 1}, vtui.AlignFill)

	// --- ПЕРВЫЙ ПРОХОД: Позиционируем контейнеры в диалоге ---
	mainVBox.Apply()
	rowTime.Apply()
	rowBtns.Apply()

	// --- ВТОРОЙ ПРОХОД: Наполняем уже спозиционированные GroupBox ---

	// Наполнение Ownership
	ownerName := strconv.Itoa(item.Uid)
	if u, err := user.LookupId(ownerName); err == nil {
		ownerName = u.Username
	}
	groupName := strconv.Itoa(item.Gid)
	if g, err := user.LookupGroupId(groupName); err == nil {
		groupName = g.Name
	}

	editOwner := vtui.NewEdit(0, 0, 20, ownerName)
	editGroup := vtui.NewEdit(0, 0, 20, groupName)

	vboxOwner := vtui.NewVBoxLayout(gbOwnership.X1+2, gbOwnership.Y1+1, gbOwnership.X2-gbOwnership.X1-4, 2)

	r1 := vtui.NewHBoxLayout(0, 0, 60, 1)
	l1 := vtui.NewLabel(0, 0, PadLabel(i18n.Msg("Attributes.Owner")), editOwner)
	r1.Add(l1, vtui.Margins{Right: 1}, vtui.AlignLeft)
	r1.Add(editOwner, vtui.Margins{}, vtui.AlignFill)
	gbOwnership.AddItem(l1)
	gbOwnership.AddItem(editOwner)
	vboxOwner.Add(r1, vtui.Margins{}, vtui.AlignFill)

	r2 := vtui.NewHBoxLayout(0, 0, 60, 1)
	l2 := vtui.NewLabel(0, 0, PadLabel(i18n.Msg("Attributes.Group")), editGroup)
	r2.Add(l2, vtui.Margins{Right: 1}, vtui.AlignLeft)
	r2.Add(editGroup, vtui.Margins{}, vtui.AlignFill)
	gbOwnership.AddItem(l2)
	gbOwnership.AddItem(editGroup)
	gbOwnership.SetFocus(false)
	vboxOwner.Add(r2, vtui.Margins{Top: 0}, vtui.AlignFill)
	vboxOwner.Apply()
	r1.Apply()
	r2.Apply()

	// Наполнение Permissions
	vboxPerms := vtui.NewVBoxLayout(gbPerms.X1+2, gbPerms.Y1+1, gbPerms.X2-gbPerms.X1-4, 5)
	allChecks := []*vtui.Checkbox{}
	threeState := len(targets) > 1

	makeRow := func(label string, bitOff uint) {
		row := vtui.NewHBoxLayout(0, 0, 60, 1)
		lbl := vtui.NewText(0, 0, PadLabel(label), vtui.Palette[vtui.ColDialogText])
		r := vtui.NewCheckbox(0, 0, i18n.Msg("Attributes.Read"), threeState)
		r.State = mixedAttributeState(targets, uint32(0400>>bitOff), func(item vfs.VFSItem) uint32 { return item.UnixMode })
		w := vtui.NewCheckbox(0, 0, i18n.Msg("Attributes.Write"), threeState)
		w.State = mixedAttributeState(targets, uint32(0200>>bitOff), func(item vfs.VFSItem) uint32 { return item.UnixMode })
		x_ := vtui.NewCheckbox(0, 0, i18n.Msg("Attributes.Execute"), threeState)
		x_.State = mixedAttributeState(targets, uint32(0100>>bitOff), func(item vfs.VFSItem) uint32 { return item.UnixMode })
		row.Add(lbl, vtui.Margins{Right: 1}, vtui.AlignLeft)
		row.Add(r, vtui.Margins{Right: 1}, vtui.AlignLeft)
		row.Add(w, vtui.Margins{Right: 1}, vtui.AlignLeft)
		row.Add(x_, vtui.Margins{}, vtui.AlignLeft)
		gbPerms.AddItem(lbl)
		gbPerms.AddItem(r)
		gbPerms.AddItem(w)
		gbPerms.AddItem(x_)
		vboxPerms.Add(row, vtui.Margins{}, vtui.AlignFill)
		allChecks = append(allChecks, r, w, x_)
		row.Apply()
	}
	makeRow(i18n.Msg("Attributes.PermUser"), 0)
	makeRow(i18n.Msg("Attributes.PermGroup"), 3)
	makeRow(i18n.Msg("Attributes.PermOther"), 6)

	editOctal := vtui.NewEdit(0, 0, 6, fmt.Sprintf("%04o", item.UnixMode))
	editOctal.Validator = &vtui.OctalValidator{MaxDigits: 4}
	editOctal.ClearSelection()
	rowOct := vtui.NewHBoxLayout(0, 0, 60, 1)
	lblOct := vtui.NewLabel(0, 0, PadLabel(i18n.Msg("Attributes.Octal")), editOctal)
	rowOct.Add(lblOct, vtui.Margins{Right: 2}, vtui.AlignLeft)
	rowOct.Add(editOctal, vtui.Margins{}, vtui.AlignLeft)
	gbPerms.AddItem(lblOct)
	gbPerms.AddItem(editOctal)
	gbPerms.SetFocus(false)
	vboxPerms.Add(rowOct, vtui.Margins{Top: 1}, vtui.AlignFill)
	vboxPerms.Apply()
	for _, itm := range vboxPerms.Items {
		if h, ok := itm.Element.(*vtui.HBoxLayout); ok {
			h.Apply()
		}
	}

	// Синхронизация (остается без изменений)
	syncing := false
	updateOct := func() {
		if syncing {
			return
		}
		syncing = true
		var m uint32
		b := []uint32{0400, 0200, 0100, 0040, 0020, 0010, 0004, 0002, 0001}
		for i, c := range allChecks {
			if c.State == 1 {
				m |= b[i]
			}
		}
		editOctal.SetText(fmt.Sprintf("%04o", m))
		syncing = false
		vtui.FrameManager.Redraw()
	}
	for _, c := range allChecks {
		c.OnChange = func(int) { updateOct() }
	}
	editOctal.OnTextChange = func(s string) {
		if syncing {
			return
		}
		var m uint64
		fmt.Sscanf(s, "%o", &m)
		syncing = true
		b := []uint32{0400, 0200, 0100, 0040, 0020, 0010, 0004, 0002, 0001}
		for i, c := range allChecks {
			if (uint32(m) & b[i]) != 0 {
				c.State = 1
			} else {
				c.State = 0
			}
		}
		syncing = false
		vtui.FrameManager.Redraw()
	}

	targetEdited := item.IsSymlink && editTarget != nil && len(targets) == 1

	btnSet.OnClick = func() {
		newTarget := ""
		if targetEdited {
			newTarget = editTarget.GetText()
		}
		uidStr := editOwner.GetText()
		if u, err := user.Lookup(uidStr); err == nil {
			item.Uid, _ = strconv.Atoi(u.Uid)
		} else {
			if parsedUid, err := strconv.Atoi(uidStr); err == nil {
				item.Uid = parsedUid
			}
		}

		gidStr := editGroup.GetText()
		if g, err := user.LookupGroup(gidStr); err == nil {
			item.Gid, _ = strconv.Atoi(g.Gid)
		} else {
			if parsedGid, err := strconv.Atoi(gidStr); err == nil {
				item.Gid = parsedGid
			}
		}
		var m uint64
		fmt.Sscanf(editOctal.GetText(), "%o", &m)
		item.UnixMode = uint32(m)
		preserveUnixMode := uint32(0)
		modeBits := []uint32{0400, 0200, 0100, 0040, 0020, 0010, 0004, 0002, 0001}
		for i, check := range allChecks {
			if check.State == 2 {
				preserveUnixMode |= modeBits[i]
			}
		}
		if t, err := time.ParseInLocation(timeFormat, editMTime.GetText(), time.Local); err == nil {
			item.MTime = t
		}
		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			if targetEdited {
				if err := ReplaceSymlinkTarget(ctx.Context, v, path, newTarget); err != nil {
					ctx.RunOnUI(func() {
						vtui.ShowMessage(" Error ", err.Error(), []string{"&Ok"})
					})
					return
				}
			}
			err := setUnixAttributesForTargets(ctx.Context, v, targets, item, preserveUnixMode)
			ctx.RunOnUI(func() {
				if err != nil {
					vtui.ShowMessage(" Error ", err.Error(), []string{"&Ok"})
				} else {
					dlg.Close()
					if refresh != nil {
						refresh()
					}
				}
			})
		})
	}
	btnCancel.OnClick = func() { dlg.Close() }
	vtui.FrameManager.Push(dlg)
}

func ShowAttributesWindows(refresh func(), v vfs.VFS, path string, item vfs.VFSItem) {
	ShowAttributesWindowsForTargets(refresh, v, []AttributesTarget{{Path: path, Item: item}})
}

func ShowAttributesWindowsForTargets(refresh func(), v vfs.VFS, targets []AttributesTarget) {
	ShowAttributesWindowsWithPropertiesForTargets(refresh, v, targets, DefaultNativePropertiesOpener)
}

func mixedAttributeState(targets []AttributesTarget, bit uint32, value func(vfs.VFSItem) uint32) int {
	if len(targets) == 0 {
		return 0
	}
	wantSet := value(targets[0].Item)&bit != 0
	for _, target := range targets[1:] {
		if (value(target.Item)&bit != 0) != wantSet {
			return 2
		}
	}
	if wantSet {
		return 1
	}
	return 0
}

var DefaultNativePropertiesOpener = showNativePropertiesOS

// ShowAttributesWindowsWithProperties keeps the native shell boundary
// injectable. In particular, UI tests must not invoke ShellExecute: it can
// outlive a test's temporary directory and make Windows display an error
// dialog after the test has already completed.
func ShowAttributesWindowsWithProperties(
	refresh func(),
	v vfs.VFS,
	path string,
	item vfs.VFSItem,
	openProperties func(string) error,
) {
	ShowAttributesWindowsWithPropertiesForTargets(refresh, v, []AttributesTarget{{Path: path, Item: item}}, openProperties)
}

func ShowAttributesWindowsWithPropertiesForTargets(
	refresh func(),
	v vfs.VFS,
	targets []AttributesTarget,
	openProperties func(string) error,
) {
	path := targets[0].Path
	item := targets[0].Item
	width, height := 60, 22
	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("Attributes.Title"))
	dlg.ShowClose = true
	x, y := dlg.X1, dlg.Y1
	const timeFormat = "02.01.2006 15:04:05"

	mainVBox := vtui.NewVBoxLayout(x+3, y+2, width-6, height-4)

	lblFile := vtui.NewText(0, 0, fmt.Sprintf(i18n.Msg("Attributes.File"), vtui.TruncateMiddle(v.Base(path), 46)), vtui.Palette[vtui.ColDialogText])
	dlg.AddItem(lblFile)
	mainVBox.Add(lblFile, vtui.Margins{}, vtui.AlignLeft)

	gbAttr := vtui.NewGroupBox(0, 0, 54, 6, " "+i18n.Msg("Attributes.Flags")+" ")
	dlg.AddItem(gbAttr)
	mainVBox.Add(gbAttr, vtui.Margins{Top: 1}, vtui.AlignFill)

	gbAdv := vtui.NewGroupBox(0, 0, 54, 3, " "+i18n.Msg("Attributes.AdvancedFlags")+" ")
	dlg.AddItem(gbAdv)
	mainVBox.Add(gbAdv, vtui.Margins{Top: 1}, vtui.AlignFill)

	editMTime := vtui.NewEdit(0, 0, 20, item.MTime.Format(timeFormat))
	lblTime := vtui.NewLabel(0, 0, PadLabel(i18n.Msg("Attributes.LastWrite")), editMTime)
	rowTime := vtui.NewHBoxLayout(0, 0, 54, 1)
	rowTime.Add(lblTime, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowTime.Add(editMTime, vtui.Margins{}, vtui.AlignLeft)
	dlg.AddItem(lblTime)
	dlg.AddItem(editMTime)
	mainVBox.Add(rowTime, vtui.Margins{Top: 1}, vtui.AlignFill)

	btnSet := vtui.NewButton(0, 0, i18n.Msg("Attributes.BtnSet"))
	btnSet.IsDefault = true
	btnSec := vtui.NewButton(0, 0, i18n.Msg("Attributes.BtnSecurity"))
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

	var osPath string
	if fileops.IsLocalOSVFS(v) {
		if abs, err := v.Abs(path); err == nil {
			if runtime.GOOS == "windows" {
				if (len(abs) >= 2 && abs[1] == ':') || strings.HasPrefix(abs, "\\\\") {
					osPath = abs
				}
			} else {
				if strings.HasPrefix(abs, "/") {
					osPath = abs
				}
			}
		}
	}
	if osPath == "" {
		btnSec.SetDisabled(true)
	}

	btnSec.OnClick = func() {
		if osPath != "" && openProperties != nil {
			if err := openProperties(osPath); err != nil {
				vtui.ShowMessage(" Error ", "Cannot open Windows properties: "+err.Error(), []string{"&Ok"})
			}
		}
	}

	rowBtns := vtui.NewHBoxLayout(0, 0, 54, 1)
	rowBtns.HorizontalAlign = vtui.AlignCenter
	rowBtns.Spacing = 2
	rowBtns.Add(btnSet, vtui.Margins{}, vtui.AlignTop)
	rowBtns.Add(btnSec, vtui.Margins{}, vtui.AlignTop)
	rowBtns.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

	dlg.AddItem(btnSet)
	dlg.AddItem(btnSec)
	dlg.AddItem(btnCancel)
	mainVBox.Add(rowBtns, vtui.Margins{Top: 1}, vtui.AlignFill)

	// Apply first pass
	mainVBox.Apply()
	rowTime.Apply()
	rowBtns.Apply()

	// Apply second pass for GroupBox
	gbVBox := vtui.NewVBoxLayout(gbAttr.X1+2, gbAttr.Y1+1, gbAttr.X2-gbAttr.X1-4, 4)
	threeState := len(targets) > 1
	chkRO := vtui.NewCheckbox(0, 0, i18n.Msg("Attributes.ReadOnly"), threeState)
	chkHD := vtui.NewCheckbox(0, 0, i18n.Msg("Attributes.Hidden"), threeState)
	chkSY := vtui.NewCheckbox(0, 0, i18n.Msg("Attributes.System"), threeState)
	chkAR := vtui.NewCheckbox(0, 0, i18n.Msg("Attributes.Archive"), threeState)

	chkRO.State = mixedAttributeState(targets, 1, func(item vfs.VFSItem) uint32 { return item.WinAttrs })
	chkHD.State = mixedAttributeState(targets, 2, func(item vfs.VFSItem) uint32 { return item.WinAttrs })
	chkSY.State = mixedAttributeState(targets, 4, func(item vfs.VFSItem) uint32 { return item.WinAttrs })
	chkAR.State = mixedAttributeState(targets, 32, func(item vfs.VFSItem) uint32 { return item.WinAttrs })

	gbAttr.AddItem(chkRO)
	gbAttr.AddItem(chkHD)
	gbAttr.AddItem(chkSY)
	gbAttr.AddItem(chkAR)
	gbVBox.Add(chkRO, vtui.Margins{}, vtui.AlignLeft)
	gbVBox.Add(chkHD, vtui.Margins{}, vtui.AlignLeft)
	gbVBox.Add(chkSY, vtui.Margins{}, vtui.AlignLeft)
	gbVBox.Add(chkAR, vtui.Margins{}, vtui.AlignLeft)
	gbVBox.Apply()

	var advFlags []string
	if (item.WinAttrs & 0x00000800) != 0 {
		advFlags = append(advFlags, "Compressed")
	}
	if (item.WinAttrs & 0x00004000) != 0 {
		advFlags = append(advFlags, "Encrypted")
	}
	if (item.WinAttrs & 0x00000400) != 0 {
		advFlags = append(advFlags, "Reparse Point")
	}
	if (item.WinAttrs & 0x00000200) != 0 {
		advFlags = append(advFlags, "Sparse")
	}
	if (item.WinAttrs & 0x00001000) != 0 {
		advFlags = append(advFlags, "Offline")
	}
	if (item.WinAttrs & 0x00002000) != 0 {
		advFlags = append(advFlags, "Not Content Indexed")
	}
	if (item.WinAttrs & 0x00000010) != 0 {
		advFlags = append(advFlags, "Directory")
	}

	advStr := "None"
	if len(advFlags) > 0 {
		advStr = strings.Join(advFlags, ", ")
	}

	lblAdv := vtui.NewText(0, 0, vtui.TruncateMiddle(advStr, 50), vtui.Palette[vtui.ColDialogText])
	gbAdv.AddItem(lblAdv)
	gbAdvVBox := vtui.NewVBoxLayout(gbAdv.X1+2, gbAdv.Y1+1, gbAdv.X2-gbAdv.X1-4, 1)
	gbAdvVBox.Add(lblAdv, vtui.Margins{}, vtui.AlignLeft)
	gbAdvVBox.Apply()

	btnSet.OnClick = func() {
		if nt, err := time.ParseInLocation(timeFormat, editMTime.GetText(), time.Local); err == nil {
			item.MTime = nt
		}

		// Real POSIX semantics apply on a genuine Unix build (runtime.GOOS
		// != "windows") and equally in Wine posix mode (hostmode.Posix());
		// on both, item.UnixMode already holds the actual rwx bits the user
		// edited via the octal field/checkboxes above, and stomping it with
		// a synthetic 0444/0666 derived from the Windows-only "read-only"
		// checkbox would silently discard real per-owner/group/other
		// permissions. Found while wiring Wine posix mode (WINE.md §14.2)
		// but the bug is not Wine-specific: item.WinAttrs is only ever
		// populated on GOOS=windows (vfs/os_vfs_windows.go), so chkRO
		// defaults to unchecked on a native Linux build too, meaning Set
		// already reset every file's mode to 0666 there before this fix.
		posixSemantics := runtime.GOOS != "windows" || hostmode.Posix()
		preserveWinAttrs := uint32(0)
		switch chkRO.State {
		case 2:
			preserveWinAttrs |= 1
		case 1:
			item.WinAttrs |= 1
			if !posixSemantics {
				item.UnixMode = 0444
			}
		default:
			item.WinAttrs &= ^uint32(1)
			if !posixSemantics {
				item.UnixMode = 0666
			}
		}
		for _, flag := range []struct {
			state int
			bit   uint32
		}{
			{chkHD.State, 2},
			{chkSY.State, 4},
			{chkAR.State, 32},
		} {
			switch flag.state {
			case 2:
				preserveWinAttrs |= flag.bit
			case 1:
				item.WinAttrs |= flag.bit
			default:
				item.WinAttrs &^= flag.bit
			}
		}

		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			err := setWindowsAttributesForTargets(ctx.Context, v, targets, item, preserveWinAttrs)
			ctx.RunOnUI(func() {
				if err != nil {
					vtui.ShowMessage(" Error ", err.Error(), []string{"&Ok"})
					return
				}
				dlg.Close()
				if refresh != nil {
					refresh()
				}
			})
		})
	}
	btnCancel.OnClick = func() { dlg.Close() }
	vtui.FrameManager.Push(dlg)
}
