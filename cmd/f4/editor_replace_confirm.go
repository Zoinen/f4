package main

// Interactive Replace, ported from Far Manager (far/editor.cpp, the
// IsReplaceMode branch of BaseSearchString): each occurrence is scrolled to
// about a quarter of the screen from the top so the centered confirmation
// dialog does not cover it, highlighted as the active selection, and
// confirmed with a Replace / All / Skip / Cancel dialog (Far's
// EditorConfirmReplaceId). [ All ] replaces the rest without further prompts
// as one undo step, and a finished loop reports how many occurrences were
// replaced.

import (
	"fmt"
	"strings"

	"github.com/coregx/coregex"
	"github.com/unxed/vtui"
)

// Confirm dialog button order; ShowMessage reports the pressed button by
// this index (Esc arrives as -1).
const (
	replaceBtnReplace = iota
	replaceBtnAll
	replaceBtnSkip
	replaceBtnCancel
)

// replaceLoop carries one interactive Replace session across its
// find → prompt → splice iterations. Fields are only touched on the UI
// thread; every find re-reads the buffer, so no offsets survive a splice
// except searchFrom, which the button handlers recompute from the occurrence
// that was just confirmed.
type replaceLoop struct {
	ev            *EditorView
	pattern       string
	replacement   string
	caseSensitive bool
	reverse       bool
	regexp        bool
	wholeWord     bool
	re            *coregex.Regex // non-nil iff regexp or wholeWord
	searchFrom    int
	replaced      int
	found         bool // at least one occurrence was prompted
	// session fences every background scan: it is captured while the loop's
	// offsets are valid (loop start, and after each of the loop's own
	// splices), so any foreign edit sneaking in between a dialog closing
	// and its posted task running is detected before splicing.
	session int
}

// renderReplacement renders the replacement for one matched span (or, with
// the whole buffer as matched, for every span in it), following the same
// rules everywhere replacement happens: $group expansion only when the user
// typed a regex. A non-nil re with typedRegex=false is whole-word mode: the
// pattern was typed as plain text, so the replacement is plain text too. A
// nil re is a literal replacement of exactly one matched span.
func renderReplacement(re *coregex.Regex, typedRegex bool, matched, replacement []byte) []byte {
	switch {
	case typedRegex:
		return re.ReplaceAll(matched, replacement)
	case re != nil:
		return re.ReplaceAllLiteral(matched, replacement)
	default:
		return replacement
	}
}

func (st *replaceLoop) renderReplacement(match []byte) []byte {
	return renderReplacement(st.re, st.regexp, match, []byte(st.replacement))
}

// selectionIsMatch reports whether sel is exactly one occurrence of the
// search pattern, using the same matching rules as findMatch.
func selectionIsMatch(sel []byte, pattern string, caseSensitive bool, re *coregex.Regex) bool {
	if re != nil {
		loc := re.FindIndex(sel)
		return loc != nil && loc[0] == 0 && loc[1] == len(sel)
	}
	if caseSensitive {
		return string(sel) == pattern
	}
	return strings.EqualFold(string(sel), pattern)
}

// findNext locates the next occurrence from searchFrom and either prompts at
// it or ends the loop. The find runs in the background under the cancelable
// progress popup; all state transitions happen back on the UI thread.
func (st *replaceLoop) findNext() {
	ev := st.ev
	vtui.FrameManager.PostTask(func() {
		session := st.session
		runSearchWithProgress(st.pattern, func(ctx *vtui.TaskContext, dlg *vtui.Window) {
			defer ev.guardMapping("replacing")()
			data, errBytes := ev.searchBuffer(ctx, session)
			if errBytes != nil {
				if ctx.Err() != nil {
					return // canceled; the dialog is already closing
				}
				ctx.RunOnUI(func() {
					dlg.Close()
					vtui.ShowMessage(" Error ", "Failed to read file buffer.", []string{"&Ok"})
				})
				return
			}

			// A zero-length regex match (e.g. "a*" before non-"a" text) is
			// not an occurrence, but text further on can still match: step
			// one byte past it and keep looking — the same way
			// findAllMatchSpans skips such matches — instead of ending the
			// loop with a false "not found".
			from := st.searchFrom
			off, mLen := -1, 0
			var err error
			for ctx.Err() == nil {
				o, l, e := findMatch(data, st.pattern, st.caseSensitive, st.reverse, st.regexp, st.wholeWord, false, from)
				if e != nil || o == -1 {
					err = e
					break
				}
				if l > 0 {
					off, mLen = o, l
					break
				}
				if st.reverse {
					from = min(o, from-1)
				} else {
					from = o + 1
				}
			}

			ctx.RunOnUI(func() {
				// Closing the dialog cancels the task via OnResult, so the
				// cancellation state must be read first: a canceled replace
				// must not touch the buffer.
				canceled := ctx.Err() != nil
				dlg.Close()
				if canceled {
					return
				}
				if err != nil {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid regular expression:\n%v", err), []string{"&Ok"})
					return
				}
				if ev.editSession != st.session {
					return // buffer changed while scanning; offsets are stale
				}
				if off == -1 {
					st.finish()
					return
				}
				st.promptAt(data, off, mLen)
			})
		})
	})
}

// promptAt shows the occurrence and the Replace / All / Skip / Cancel dialog
// for it. Runs on the UI thread; the dialog is modal, so the buffer cannot
// change between the prompt and the button handler.
func (st *replaceLoop) promptAt(data []byte, off, mLen int) {
	ev := st.ev
	st.found = true
	match := append([]byte(nil), data[off:off+mLen]...)
	rendered := st.renderReplacement(match)

	ev.selectFoundPattern(off, mLen)
	ev.scrollFoundToQuarter(off)
	vtui.FrameManager.Redraw()

	buttons := []string{
		Msg("Replace.BtnReplace"),
		Msg("Replace.BtnAll"),
		Msg("Replace.BtnSkip"),
		Msg("vtui.Cancel"),
	}
	dlg := vtui.ShowMessageEx(Msg("Replace.ConfirmTitle"),
		replacePromptBody(string(match), string(rendered)), buttons, vtui.MessageInfo)

	// SetExitCode fires OnResult on every close, and a done dialog can be
	// closed again during frame cleanup; only the first verdict counts.
	handled := false
	dlg.OnResult = func(code int) {
		if handled {
			return
		}
		handled = true
		switch code {
		case replaceBtnReplace:
			ev.replaceRange(off, off+mLen, rendered)
			st.session = ev.editSession
			st.replaced++
			if st.reverse {
				// A splice changes nothing left of its start; continue
				// strictly before the replaced occurrence.
				st.searchFrom = off
			} else {
				// Continue from the end of the replacement (next=false
				// semantics): an adjacent match starting exactly there must
				// not be skipped, while the replacement's own output is
				// never re-matched.
				st.searchFrom = off + len(rendered)
			}
			st.findNext()
		case replaceBtnAll:
			st.replaceRemaining(off, mLen)
		case replaceBtnSkip:
			if st.reverse {
				st.searchFrom = off
			} else {
				st.searchFrom = off + mLen
			}
			st.findNext()
		default:
			// Cancel or Esc: stop the loop; the occurrence stays selected
			// with the cursor on it, and no summary is shown (Far does the
			// same on cancel).
		}
	}
}

// replaceRemaining implements [ All ]: the confirmed occurrence and every
// one the loop would still visit are collected in one linear pass over a
// buffer snapshot — the same scan Find All uses — and applied as per-span
// splices under a single undo step and one repaint.
func (st *replaceLoop) replaceRemaining(off, mLen int) {
	ev := st.ev
	vtui.FrameManager.PostTask(func() {
		session := st.session
		runSearchWithProgress(st.pattern, func(ctx *vtui.TaskContext, dlg *vtui.Window) {
			defer ev.guardMapping("replacing")()
			data, errBytes := ev.searchBuffer(ctx, session)
			if errBytes != nil {
				if ctx.Err() != nil {
					return // canceled; the dialog is already closing
				}
				ctx.RunOnUI(func() {
					dlg.Close()
					vtui.ShowMessage(" Error ", "Failed to read file buffer.", []string{"&Ok"})
				})
				return
			}

			// The confirmed occurrence plus — forward — everything after
			// it, or — reverse — everything before it (the loop walks right
			// to left). findAllMatchSpans returns ascending spans, which is
			// what the splice wants, and skips zero-length matches, so an
			// empty regex match cannot truncate the collection.
			var spans []matchSpan
			var err error
			if st.reverse {
				spans, err = findAllMatchSpans(ctx, data[:off], st.pattern, st.caseSensitive, st.regexp, st.wholeWord)
				spans = append(spans, matchSpan{off, mLen})
			} else {
				tail, tailErr := findAllMatchSpans(ctx, data[off+mLen:], st.pattern, st.caseSensitive, st.regexp, st.wholeWord)
				err = tailErr
				spans = append(spans, matchSpan{off, mLen})
				for _, s := range tail {
					spans = append(spans, matchSpan{s.Off + off + mLen, s.Len})
				}
			}

			renders := make([][]byte, len(spans))
			replacement := []byte(st.replacement)
			for i, s := range spans {
				renders[i] = renderReplacement(st.re, st.regexp, data[s.Off:s.Off+s.Len], replacement)
			}

			ctx.RunOnUI(func() {
				canceled := ctx.Err() != nil
				dlg.Close()
				if canceled {
					return
				}
				if err != nil {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid regular expression:\n%v", err), []string{"&Ok"})
					return
				}
				if ev.editSession != st.session {
					return // buffer changed while scanning; offsets are stale
				}
				ev.replaceSpans(spans, renders)
				st.replaced += len(spans)
				st.finish()
			})
		})
	})
}

// finish ends the loop: a summary once any occurrence was prompted —
// "0 occurrence(s) replaced" is the honest outcome of skipping every match —
// and the not-found message only when the very first find came up empty.
func (st *replaceLoop) finish() {
	if st.found {
		vtui.ShowMessage(Msg("Replace.ConfirmTitle"),
			fmt.Sprintf(Msg("Replace.Replaced"), st.replaced), []string{Msg("vtui.Ok")})
	} else {
		vtui.ShowMessage(Msg("Replace.ConfirmTitle"), Msg("Search.NotFound"), []string{Msg("vtui.Ok")})
	}
}

// replacePromptBody builds the four-line confirm dialog body: like Far, the
// found text and the concrete rendered replacement are shown quoted, each on
// its own line.
func replacePromptBody(match, rendered string) string {
	return Msg("Replace.AskReplace") + "\n" + promptQuote(match) + "\n" +
		Msg("Replace.AskWith") + "\n" + promptQuote(rendered)
}

// promptQuote renders one side of the confirm dialog body: whitespace
// control characters become plain spaces so the string stays on one line,
// long text is truncated to keep the message box inside its width cap, and
// the result is quoted like Far's quote_unconditional.
func promptQuote(s string) string {
	s = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)
	return "\"" + vtui.TruncateString(s, 60, "…") + "\""
}

// scrollFoundToQuarter scrolls the view so the found line sits about a
// quarter of the editor height from the top ("Отступим на четверть" in
// far/editor.cpp): the centered confirm dialog then leaves it visible.
func (ev *EditorView) scrollFoundToQuarter(off int) {
	height := ev.Y2 - ev.Y1
	if height <= 0 {
		return
	}
	vRow, _ := ev.engine.LogicalToVisual(off)
	top := max(vRow-height/4, 0)
	maxTop := max(ev.engine.GetTotalVisualRows()-height, 0)
	ev.ScrollTopRow = min(top, maxTop)
}
