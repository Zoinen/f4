package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/vtui"
)

// F8 on the patch review screen: turn an edit down with a reason
// (f4#1606, docs/VTVIBE.md §7.3 and appendix B.2).
//
// A rejected edit is left out of Apply and Dry run exactly like an edit
// switched off with Space, and in addition one line about it goes into the
// draft of the next message to the model: which edit (its number on the
// review screen, file, action and what it searched for) and the reason the
// human typed. Rejections pile up in one list, a draft section f4 rewrites
// as a whole (vtvibe.Session.SetDraftSection), so F8 on an edit that is
// already rejected edits its reason or takes the rejection back without
// leaving a stale line behind.
//
// The list belongs to one patch. It is forgotten when the model sends a new
// one, and when the draft no longer holds it - sent, or cut out by hand.

// aiRejectSection is the name of the draft section with the rejections.
const aiRejectSection = "rejected-edits"

// aiReject is one rejected edit, with everything its draft line needs.
type aiReject struct {
	n       int // 1-based row on the review screen
	file    string
	action  string
	locator string
	reason  string
}

// aiRejected is the rejection list of the patch being reviewed. The review
// dialog is rebuilt after every dry run, so the list cannot live in it.
var aiRejected struct {
	patch *vtvibe.Patch
	byKey map[ap.ModKey]aiReject
}

// aiReviewSession is the session whose draft the rejections go to; a
// variable only so tests get a session of their own.
var aiReviewSession = aiSession

// aiRejectionsFor returns the rejection list of patch, starting a new one
// when the list on hand belongs to another patch or the draft lost it.
func aiRejectionsFor(patch *vtvibe.Patch) map[ap.ModKey]aiReject {
	if aiRejected.patch != patch || aiRejected.byKey == nil ||
		(len(aiRejected.byKey) > 0 && !aiReviewSession().HasDraftSection(aiRejectSection)) {
		aiRejected.patch = patch
		aiRejected.byKey = map[ap.ModKey]aiReject{}
	}
	return aiRejected.byKey
}

// aiRejectDraftText renders the rejection list for the draft: a header
// naming the patch, then one line per edit in screen order.
func aiRejectDraftText(patch *vtvibe.Patch, byKey map[ap.ModKey]aiReject) string {
	if len(byKey) == 0 {
		return ""
	}
	list := make([]aiReject, 0, len(byKey))
	for _, r := range byKey {
		list = append(list, r)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].n < list[j].n })
	id := "-"
	if patch != nil && patch.ID != "" {
		id = patch.ID
	}
	var b strings.Builder
	fmt.Fprintf(&b, i18n.Msg("AI.RejectDraftHeader"), id)
	for _, r := range list {
		b.WriteString("\n")
		b.WriteString(aiRejectLine(r))
	}
	return b.String()
}

// aiRejectSubject is "file, ACTION «locator»": enough for the model to
// find its own edit in the patch it sent.
func aiRejectSubject(r aiReject) string {
	what := r.file
	if r.action != "" {
		what += ", " + r.action
	}
	if r.locator != "" {
		what += " «" + r.locator + "»"
	}
	return what
}

// aiRejectLine is "edit N (file, ACTION «locator») rejected: reason".
func aiRejectLine(r aiReject) string {
	what := aiRejectSubject(r)
	reason := strings.TrimSpace(r.reason)
	if reason == "" {
		reason = i18n.Msg("AI.RejectNoReason")
	}
	return fmt.Sprintf(i18n.Msg("AI.RejectDraftLine"), r.n, what, reason)
}

// aiRejectOf is row i of the review as a rejection with the given reason.
func aiRejectOf(i int, m ap.ModificationResult, reason string) aiReject {
	return aiReject{n: i + 1, file: m.FilePath, action: m.Action, locator: aiReviewOneLine(m.Locator), reason: reason}
}

// aiShowRejectDialog asks for the reason. reason is the current one when
// the edit is already rejected; then there is also a button that takes the
// rejection back.
func aiShowRejectDialog(what, reason string, rejected bool, onReject func(string), onUndo func()) *vtui.Window {
	scrW := vtui.FrameManager.GetScreenSize()
	w := min(max(scrW-10, 50), 76)
	h := 10
	inner := w - 4
	dlg := vtui.NewCenteredDialog(w, h, i18n.Msg("AI.RejectTitle"))
	dlg.ShowClose = true

	whatText := vtui.NewText(0, 0, aiReviewLabel(what, inner), 0)
	edit := vtui.NewEdit(0, 0, inner, reason)
	prompt := vtui.NewLabel(0, 0, i18n.Msg("AI.RejectPrompt"), edit)

	var buttons []*vtui.Button
	add := func(label string, onClick func()) *vtui.Button {
		b := vtui.NewButton(0, 0, label)
		b.SetOwner(dlg)
		b.OnClick = onClick
		buttons = append(buttons, b)
		return b
	}
	ok := add(i18n.Msg("AI.RejectBtn"), func() {
		dlg.SetExitCode(1)
		onReject(strings.TrimSpace(edit.GetText()))
	})
	ok.IsDefault = true
	if rejected {
		add(i18n.Msg("AI.RejectBtnUndo"), func() {
			dlg.SetExitCode(2)
			onUndo()
		})
	}
	add(i18n.Msg("vtui.Cancel"), func() { dlg.SetExitCode(-1) })

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+1, inner, h-2)
	vbox.Add(whatText, vtui.Margins{Bottom: 1}, vtui.AlignFill)
	vbox.Add(prompt, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(edit, vtui.Margins{Bottom: 1}, vtui.AlignFill)
	btnRow := vtui.NewHBoxLayout(0, 0, inner, 1)
	btnRow.HorizontalAlign = vtui.AlignCenter
	btnRow.Spacing = 2
	for _, b := range buttons {
		btnRow.Add(b, vtui.Margins{}, vtui.AlignTop)
	}
	vbox.Add(btnRow, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	dlg.AddItem(whatText)
	dlg.AddItem(prompt)
	dlg.AddItem(edit)
	for _, b := range buttons {
		dlg.AddItem(b)
	}
	vtui.FrameManager.Push(dlg)
	return dlg
}
