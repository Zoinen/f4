package main

import (
	"fmt"
	"time"

	"github.com/unxed/vtui"
)

// ShowBackgroundJobs lists the work that is still running and the answers
// that are waiting to be looked at. It is the way back to a job whose window
// was closed, and the only way to see a result that finished while nobody
// was watching.
func ShowBackgroundJobs(pf *PanelsFrame) {
	width, height := 66, 16
	dlg := vtui.NewCenteredDialog(width, height, Msg("Jobs.Title"))
	dlg.ShowClose = true

	lb := vtui.NewListBox(0, 0, width-4, height-6, nil)
	btnShow := vtui.NewButton(0, 0, Msg("Jobs.BtnShow"))
	btnCancel := vtui.NewButton(0, 0, Msg("Jobs.BtnCancel"))
	btnClose := vtui.NewButton(0, 0, Msg("Jobs.BtnClose"))

	dlg.AddItem(lb)
	dlg.AddItem(btnShow)
	dlg.AddItem(btnCancel)
	dlg.AddItem(btnClose)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(lb, vtui.Margins{Bottom: 1}, vtui.AlignFill)
	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnShow, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnClose, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	// ids parallels the list, because a row can disappear between one redraw
	// and the next and an index alone would then point at somebody else.
	var ids []int
	refresh := func() {
		states := GlobalBackgroundJobs.List()
		items := make([]string, 0, len(states))
		ids = ids[:0]
		for _, s := range states {
			mark := "running "
			if s.Done {
				mark = "finished"
			}
			line := fmt.Sprintf("%s  %s", mark, s.Title)
			if s.Status != "" {
				line += "  —  " + s.Status
			}
			line += fmt.Sprintf("  (%s)", time.Since(s.Started).Round(time.Second))
			items = append(items, line)
			ids = append(ids, s.ID)
		}
		if len(items) == 0 {
			items = append(items, Msg("Jobs.Empty"))
		}
		lb.Items = items
		lb.UpdateRows()
	}
	refresh()

	selected := func() int {
		idx := lb.SelectPos
		if idx < 0 || idx >= len(ids) {
			return 0
		}
		return ids[idx]
	}

	btnShow.OnClick = func() {
		id := selected()
		if id == 0 {
			return
		}
		if GlobalBackgroundJobs.Open(id) {
			dlg.Close()
			return
		}
		refresh()
		vtui.FrameManager.Redraw()
	}
	btnCancel.OnClick = func() {
		if id := selected(); id != 0 {
			GlobalBackgroundJobs.Cancel(id)
		}
		refresh()
		vtui.FrameManager.Redraw()
	}
	btnClose.OnClick = func() { dlg.Close() }
	lb.OnAction = func(int) { btnShow.OnClick() }

	vtui.FrameManager.Push(dlg)
}
