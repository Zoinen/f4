package main

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"time"

	"os/user"
	"strconv"

	"github.com/mattn/go-runewidth"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func padLabel(s string) string {
	for runewidth.StringWidth(s) < 12 {
		s += " "
	}
	return s
}
func isLocalOSVFS(v any) bool {
	if v == nil {
		return false
	}
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Interface {
		val = val.Elem()
	}
	if val.Kind() == reflect.Ptr {
		if _, ok := val.Interface().(*vfs.OSVFS); ok {
			return true
		}
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			if field.CanInterface() {
				if isLocalOSVFS(field.Interface()) {
					return true
				}
			}
		}
	}
	return false
}

func ShowAttributesDialog(pf *PanelsFrame, v vfs.VFS, path string, item vfs.VFSItem) {
	caps := v.GetCapabilities()
	if !caps.HasUnixPermissions {
		showAttributesWindows(pf, v, path, item)
	} else {
		showAttributesUnix(pf, v, path, item)
	}
}

func showAttributesUnix(pf *PanelsFrame, v vfs.VFS, path string, item vfs.VFSItem) {
	// Увеличиваем высоту до 24, чтобы кнопкам было просторно
	width, height := 70, 24

	dlg := vtui.NewCenteredDialog(width, height, " Attributes ")
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

	// Ownership Group
	gbOwnership := vtui.NewGroupBox(0, 0, 66, 4, " Ownership ")
	dlg.AddItem(gbOwnership)
	mainVBox.Add(gbOwnership, vtui.Margins{Top: 1}, vtui.AlignFill)

	// Permissions Group
	// Permissions Group
	gbPerms := vtui.NewGroupBox(0, 0, 66, 7, " Permissions ")
	dlg.AddItem(gbPerms)
	mainVBox.Add(gbPerms, vtui.Margins{Top: 0}, vtui.AlignFill)

	// Time Row
	editMTime := vtui.NewEdit(0, 0, 20, item.MTime.Format(timeFormat))
	lblTime := vtui.NewLabel(0, 0, padLabel("M-Time:"), editMTime)
	rowTime := vtui.NewHBoxLayout(0, 0, 66, 1)
	rowTime.Add(lblTime, vtui.Margins{Left: 2, Right: 1}, vtui.AlignLeft)
	rowTime.Add(editMTime, vtui.Margins{}, vtui.AlignLeft)
	dlg.AddItem(lblTime)
	dlg.AddItem(editMTime)
	mainVBox.Add(rowTime, vtui.Margins{Top: 0}, vtui.AlignFill)

	// Buttons
	btnSet := vtui.NewButton(0, 0, "Set")
	btnSet.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, "Cancel")
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
	l1 := vtui.NewLabel(0, 0, padLabel("Owne&r:"), editOwner)
	r1.Add(l1, vtui.Margins{Right: 1}, vtui.AlignLeft)
	r1.Add(editOwner, vtui.Margins{}, vtui.AlignFill)
	gbOwnership.AddItem(l1)
	gbOwnership.AddItem(editOwner)
	vboxOwner.Add(r1, vtui.Margins{}, vtui.AlignFill)

	r2 := vtui.NewHBoxLayout(0, 0, 60, 1)
	l2 := vtui.NewLabel(0, 0, padLabel("&Group:"), editGroup)
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

	makeRow := func(label string, bitOff uint) {
		row := vtui.NewHBoxLayout(0, 0, 60, 1)
		lbl := vtui.NewText(0, 0, padLabel(label), vtui.Palette[vtui.ColDialogText])
		r := vtui.NewCheckbox(0, 0, "Read", false)
		r.State = map[bool]int{true: 1}[(item.UnixMode&(0400>>bitOff)) != 0]
		w := vtui.NewCheckbox(0, 0, "Write", false)
		w.State = map[bool]int{true: 1}[(item.UnixMode&(0200>>bitOff)) != 0]
		x_ := vtui.NewCheckbox(0, 0, "Execute", false)
		x_.State = map[bool]int{true: 1}[(item.UnixMode&(0100>>bitOff)) != 0]
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
	makeRow("User:", 0)
	makeRow("Group:", 3)
	makeRow("Other:", 6)

	editOctal := vtui.NewEdit(0, 0, 6, fmt.Sprintf("%04o", item.UnixMode))
	editOctal.Validator = &vtui.OctalValidator{MaxDigits: 4}
	editOctal.ClearSelection()
	rowOct := vtui.NewHBoxLayout(0, 0, 60, 1)
	lblOct := vtui.NewLabel(0, 0, padLabel("O&ct:"), editOctal)
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

	btnSet.OnClick = func() {
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
		if t, err := time.ParseInLocation(timeFormat, editMTime.GetText(), time.Local); err == nil {
			item.MTime = t
		}
		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			err := v.SetAttributes(ctx.Context, path, item)
			ctx.RunOnUI(func() {
				if err != nil {
					vtui.ShowMessage(" Error ", err.Error(), []string{"&Ok"})
				} else {
					dlg.Close()
					pf.RefreshAll()
				}
			})
		})
	}
	btnCancel.OnClick = func() { dlg.Close() }
	vtui.FrameManager.Push(dlg)
}

func showAttributesWindows(pf *PanelsFrame, v vfs.VFS, path string, item vfs.VFSItem) {
	width, height := 60, 20
	dlg := vtui.NewCenteredDialog(width, height, " Attributes ")
	dlg.ShowClose = true
	x, y := dlg.X1, dlg.Y1
	const timeFormat = "02.01.2006 15:04:05"

	mainVBox := vtui.NewVBoxLayout(x+3, y+1, width-6, height-3)

	lblFile := vtui.NewText(0, 0, "File: "+vtui.TruncateMiddle(v.Base(path), 46), vtui.Palette[vtui.ColDialogText])
	dlg.AddItem(lblFile)
	mainVBox.Add(lblFile, vtui.Margins{}, vtui.AlignLeft)

	gbAttr := vtui.NewGroupBox(0, 0, 54, 6, " Flags ")
	dlg.AddItem(gbAttr)
	mainVBox.Add(gbAttr, vtui.Margins{Top: 1}, vtui.AlignFill)

	gbAdv := vtui.NewGroupBox(0, 0, 54, 3, " Advanced NTFS Flags ")
	dlg.AddItem(gbAdv)
	mainVBox.Add(gbAdv, vtui.Margins{Top: 1}, vtui.AlignFill)

	editMTime := vtui.NewEdit(0, 0, 20, item.MTime.Format(timeFormat))
	lblTime := vtui.NewLabel(0, 0, padLabel("Last write:"), editMTime)
	rowTime := vtui.NewHBoxLayout(0, 0, 54, 1)
	rowTime.Add(lblTime, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowTime.Add(editMTime, vtui.Margins{}, vtui.AlignLeft)
	dlg.AddItem(lblTime)
	dlg.AddItem(editMTime)
	mainVBox.Add(rowTime, vtui.Margins{Top: 1}, vtui.AlignFill)

	btnSet := vtui.NewButton(0, 0, "Set")
	btnSet.IsDefault = true
	btnSec := vtui.NewButton(0, 0, "&Security")
	btnCancel := vtui.NewButton(0, 0, "Cancel")

	var osPath string
	if isLocalOSVFS(v) {
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
		if osPath != "" {
			showNativePropertiesOS(osPath)
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
	chkRO := vtui.NewCheckbox(0, 0, "&Read only", false)
	chkHD := vtui.NewCheckbox(0, 0, "&Hidden", false)
	chkSY := vtui.NewCheckbox(0, 0, "&System", false)
	chkAR := vtui.NewCheckbox(0, 0, "&Archive", false)

	if (item.WinAttrs & 1) != 0 {
		chkRO.State = 1
	}
	if (item.WinAttrs & 2) != 0 {
		chkHD.State = 1
	}
	if (item.WinAttrs & 4) != 0 {
		chkSY.State = 1
	}
	if (item.WinAttrs & 32) != 0 {
		chkAR.State = 1
	}

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

		if chkRO.State == 1 {
			item.WinAttrs |= 1
			item.UnixMode = 0444
		} else {
			item.WinAttrs &= ^uint32(1)
			item.UnixMode = 0666
		}
		if chkHD.State == 1 {
			item.WinAttrs |= 2
		} else {
			item.WinAttrs &= ^uint32(2)
		}
		if chkSY.State == 1 {
			item.WinAttrs |= 4
		} else {
			item.WinAttrs &= ^uint32(4)
		}
		if chkAR.State == 1 {
			item.WinAttrs |= 32
		} else {
			item.WinAttrs &= ^uint32(32)
		}

		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			v.SetAttributes(ctx.Context, path, item)
			ctx.RunOnUI(func() { dlg.Close(); pf.RefreshAll() })
		})
	}
	btnCancel.OnClick = func() { dlg.Close() }
	vtui.FrameManager.Push(dlg)
}
