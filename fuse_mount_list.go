package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/unxed/f4/fusefs"
	"github.com/unxed/vtui"
)

// The mounts dialog (FUSE.md, iteration 2): one list over the live mounts,
// with Go to and Unmount on the selected one.
//
// It lists the mounts this process owns — the ones the panel command makes.
// Mounts started from a shell live in the cross-process registry and are a
// separate step.
func init() {
	RegisterAction(Action{
		Name:        "Panel.MountList",
		Area:        "Shell",
		Label:       "FUSE Mounts",
		Description: "List the live FUSE mounts, go to one or unmount it",
		DefaultKeys: []string{"CtrlAltL"},
		MenuPath:    "Commands",
		Visible:     fusefs.Supported,
		Handler: func() bool {
			pf := findPanelsFrameAnyScreen()
			if pf == nil {
				return false
			}
			showMountList(pf)
			return true
		},
	})
}

// mountRow is one line of the dialog: a mount this process owns, or a record
// of one some other f4 owns.
type mountRow struct {
	point  string
	source string
	mode   string
	age    time.Duration
	note   string
	live   *fusefs.Mount // nil when the mount belongs to another process
}

// mountRows lists what is mounted right now: this process's mounts first,
// then the registry records that describe everybody else's — a mount started
// from a shell or by fstab is invisible otherwise.
func mountRows() []mountRow {
	var rows []mountRow
	seen := make(map[string]bool)
	for _, m := range fusefs.List() {
		rows = append(rows, mountRow{
			point:  m.MountPoint,
			source: m.Source,
			mode:   mountMode(m.ReadOnly),
			age:    time.Since(m.Since).Truncate(time.Second),
			live:   m,
		})
		seen[m.MountPoint] = true
	}
	recs, err := fusefs.Mounts()
	if err != nil {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].age > rows[j].age })
		return rows
	}
	for _, r := range recs {
		if seen[r.MountPoint] {
			continue
		}
		rows = append(rows, mountRow{
			point:  r.MountPoint,
			source: r.Source,
			mode:   r.Mode(),
			age:    r.Age().Truncate(time.Second),
			note:   fmt.Sprintf(" (pid %d)", r.PID),
		})
	}
	// Oldest first, the way fusefs.List() orders its own: a merged list that
	// kept this process's mounts on top would read as two lists stacked.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].age > rows[j].age })
	return rows
}

func showMountList(pf *PanelsFrame) {
	rows := mountRows()
	if len(rows) == 0 {
		// An empty list is the most likely moment for someone to want
		// their first mount, so offer it here instead of sending them
		// back to the menu.
		dlg := vtui.ShowMessage(Msg("Mounts.Title"), "Nothing is mounted.",
			[]string{"&Mount this panel", "Mount read-&write", "&Ok"})
		dlg.OnResult = func(code int) {
			switch code {
			case 0:
				mountActivePanel(pf, true)
			case 1:
				mountActivePanel(pf, false)
			}
		}
		return
	}

	w, h := 70, len(rows)+2
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	if maxH := scrH - 2; h > maxH && maxH >= 5 {
		h = maxH
	}
	if w > scrW-2 {
		w = scrW - 2
	}

	here := ""
	if fsp := pf.getActivePanel(); fsp != nil && fsp.vfs != nil {
		here = fsp.vfs.GetPath()
	}

	menu := vtui.NewVMenu(Msg("Mounts.Title"))
	if live := liveRows(rows); len(live) > 1 {
		menu.AddItem(vtui.MenuItem{
			Text:     fmt.Sprintf("Unmount all (%d)", len(live)),
			UserData: -1,
		})
	}
	for i, r := range rows {
		menu.AddItem(vtui.MenuItem{
			// A remote source can be far longer than the dialog; a row
			// that overflows hides the mount point, which is the one
			// thing the row exists to show.
			// The mount the panel is standing in is the one whose
			// unmount will move the panel, so say which it is.
			Text: vtui.TruncateMiddle(fmt.Sprintf("%s  %s %s  \u2190  %s%s%s",
				r.point, r.mode, r.age, r.source, r.note, hereMark(here, r.point)), w-4),
			UserData: i,
		})
	}

	x, y := (scrW-w)/2, (scrH-h)/2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	menu.SetPosition(x, y, x+w-1, y+h-1)

	menu.OnAction = func(idx int) {
		menu.Close()
		if idx < 0 || idx >= len(menu.Items) {
			return
		}
		i, ok := menu.Items[idx].UserData.(int)
		if !ok {
			return
		}
		if i < 0 {
			// Same courtesy as a single unmount: our own panel must
			// not be the thing that makes the unmount fail.
			if fsp := pf.getActivePanel(); fsp != nil && fsp.vfs != nil {
				for _, r := range liveRows(rows) {
					if withinMount(fsp.vfs.GetPath(), r.point) {
						pf.NavigateToPath(fsp, filepath.Dir(r.point))
						break
					}
				}
			}
			unmountAll(rows)
			// Same as a single unmount: if anything is still up, the
			// list is where the user was.
			if len(mountRows()) > 0 {
				showMountList(pf)
			}
			return
		}
		if i >= len(rows) {
			return
		}
		askMountAction(pf, rows[i])
	}
	vtui.FrameManager.Push(menu)
}

// askMountAction offers what can be done to the selected mount. Unmount does
// not force: a busy mount is a question for the user ("something is still in
// there"), not an error to paper over. A mount owned by another process can
// only be visited from here — taking it down is what f4 --umount is for.
func askMountAction(pf *PanelsFrame, row mountRow) {
	buttons := []string{"&Go to", "&Unmount", "&Cancel"}
	if row.live == nil {
		buttons = []string{"&Go to", "&Cancel"}
	}
	// The point alone left the user checking it against the list; the row's
	// own words are what they picked.
	body := fmt.Sprintf("%s\n%s  %s%s", row.point, row.mode, row.source, row.note)
	dlg := vtui.ShowMessage(" Mount ", body, buttons)
	dlg.OnResult = func(code int) {
		if code == 0 {
			if fsp := pf.getActivePanel(); fsp != nil {
				pf.NavigateToPath(fsp, row.point)
			}
			return
		}
		if code == 1 && row.live != nil {
			// A panel standing inside the mount is a program holding
			// it open, and the unmount would come back EBUSY because
			// of us. Step out first: the user asked for the mount to
			// go, not for an explanation of why it cannot.
			if fsp := pf.getActivePanel(); fsp != nil && withinMount(fsp.vfs.GetPath(), row.point) {
				pf.NavigateToPath(fsp, filepath.Dir(row.point))
			}
			if err := row.live.Unmount(); err != nil {
				vtui.ShowMessage(" Mount ", fmt.Sprintf("Cannot unmount %s:\n%v", row.point, err), []string{"&Ok"})
				return
			}
			// Unmounting several is one gesture repeated, not a reason
			// to reopen the dialog by hand each time.
			if len(mountRows()) > 0 {
				showMountList(pf)
			}
		}
	}
}

// liveRows are the mounts this process can actually take down.
func liveRows(rows []mountRow) []mountRow {
	var live []mountRow
	for _, r := range rows {
		if r.live != nil {
			live = append(live, r)
		}
	}
	return live
}

// unmountAll takes down every mount this process owns and reports the ones
// that would not go, rather than stopping at the first. A busy mount is not a
// reason to leave the others up.
func unmountAll(rows []mountRow) {
	var failed []string
	for _, r := range liveRows(rows) {
		if err := r.live.Unmount(); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", r.point, err))
		}
	}
	if len(failed) > 0 {
		vtui.ShowMessage(Msg("Mounts.Title"), "Still mounted:\n"+strings.Join(failed, "\n"), []string{"&Ok"})
	}
}

// mountMode renders the access mode the way mount(8) output does, so a mount
// this process owns and a record from the registry read the same.
func mountMode(readOnly bool) string {
	if readOnly {
		return "ro"
	}
	return "rw"
}

// withinMount reports whether a panel path sits inside a mount point.
func withinMount(panelPath, mountPoint string) bool {
	if panelPath == "" || mountPoint == "" {
		return false
	}
	if panelPath == mountPoint {
		return true
	}
	return strings.HasPrefix(panelPath, mountPoint+string(filepath.Separator))
}

// hereMark labels the mount the active panel is inside.
func hereMark(panelPath, mountPoint string) string {
	if withinMount(panelPath, mountPoint) {
		return "  (here)"
	}
	return ""
}
