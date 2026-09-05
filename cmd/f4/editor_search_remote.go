package main

import (
	"bytes"
	"context"

	"github.com/charlievieth/strcase"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// searchDelegate is what the editor needs to ask a file system to do its own
// searching, captured on the UI thread so the background scan touches none of
// the editor's fields.
type searchDelegate struct {
	fs   vfs.VFS
	path string
}

// searchDelegation decides whether this search can be handed to the file
// system, and returns what the worker needs if it can.
//
// Reading a remote file to search it means downloading it. A host that can grep
// its own copy answers in one round trip whatever the size, which is the whole
// reason vfs.Search exists — the viewer has used it for a while, the editor
// never has.
//
// Four things have to hold. The file system must offer the capability. The
// buffer must still be the file: the offsets come back as positions in what is
// on disk, and an edit moves the text away from that. The content must be raw
// UTF-8, since a decoded buffer's offsets are not the file's either. And the
// pattern must be one a fixed-string grep can express, which rules out regular
// expressions and whole-word matching.
//
// Call it on the UI thread.
func (ev *EditorView) searchDelegation(useRegex, wholeWord bool) (searchDelegate, bool) {
	if ev.vfs == nil || ev.filePath == "" || useRegex || wholeWord {
		return searchDelegate{}, false
	}
	if ev.Codepage != 65001 {
		return searchDelegate{}, false
	}
	if !ev.vfs.GetCapabilities().HasSearch {
		return searchDelegate{}, false
	}
	// GetState against the state the file was last read or written at is the
	// exact question "is this buffer still the file on disk".
	if !ev.pt.GetState().Equals(ev.cleanState) {
		return searchDelegate{}, false
	}
	return searchDelegate{fs: ev.vfs, path: ev.filePath}, true
}

// searchViaFileSystem asks the file system for the occurrences and returns the
// one this search wants. handled is false whenever the caller should scan the
// buffer itself, which covers every uncertainty: no answer, an error, or a list
// that contains nothing usable.
//
// That last case matters more than it looks. A host-side search caps how many
// matches it reports — FISH+ stops at ten thousand — so a list can be short of
// the truth. Coming up empty therefore cannot mean "not in the file"; it means
// "ask the buffer", which is slower and certain.
func (ev *EditorView) searchViaFileSystem(ctx context.Context, d searchDelegate, pattern string,
	caseSensitive, reverse, next bool, startOff int) (int, int, bool) {

	matches, err := d.fs.Search(ctx, d.path, pattern)
	if err != nil || matches == nil {
		return 0, 0, false
	}

	var offsets []int
	for at := range matches {
		if at >= 0 {
			offsets = append(offsets, int(at))
		}
	}
	if len(offsets) == 0 {
		return 0, 0, false
	}

	// The host folds case and matches fixed strings, so what comes back is a
	// superset of what this search wants. Each candidate is confirmed against
	// the buffer's own rules before it is used.
	if !reverse {
		from := startOff
		if next {
			from++
		}
		best, bestLen, found := 0, 0, false
		for _, off := range offsets {
			if off < from {
				continue
			}
			if found && off >= best {
				continue
			}
			if mLen, ok := ev.confirmMatchAt(off, pattern, caseSensitive); ok {
				best, bestLen, found = off, mLen, true
			}
		}
		if found {
			return best, bestLen, true
		}
		return 0, 0, false
	}

	until := startOff
	if next {
		until--
	}
	best, bestLen, found := 0, 0, false
	for _, off := range offsets {
		if off >= until || (found && off <= best) {
			continue
		}
		if mLen, ok := ev.confirmMatchAt(off, pattern, caseSensitive); ok && off+mLen <= until {
			best, bestLen, found = off, mLen, true
		}
	}
	if found {
		return best, bestLen, true
	}
	return 0, 0, false
}

// confirmMatchAt checks that the pattern really starts at off under this
// search's own rules, and reports how many bytes it covers there. A folded
// match can be longer or shorter than the pattern — K U+212A matches "k" — so
// the length is measured rather than assumed.
func (ev *EditorView) confirmMatchAt(off int, pattern string, caseSensitive bool) (int, bool) {
	if off < 0 || pattern == "" {
		return 0, false
	}
	// Four bytes of buffer per pattern byte is enough for any folding that
	// grows what it matches, and costs one small read.
	window := len(pattern) * 4
	if window < len(pattern)+8 {
		window = len(pattern) + 8
	}
	if off+window > ev.pt.Size() {
		window = ev.pt.Size() - off
	}
	if window <= 0 {
		return 0, false
	}

	data, err := ev.pt.GetRange(off, window)
	if err != nil || len(data) == 0 {
		return 0, false
	}

	if caseSensitive {
		if bytes.HasPrefix(data, []byte(pattern)) {
			return len(pattern), true
		}
		return 0, false
	}
	text := bytesToString(data)
	after, ok := strcase.CutPrefix(text, pattern)
	if !ok {
		return 0, false
	}
	return len(text) - len(after), true
}

// logSearchDelegation records which way a search went, which is the one thing
// that is hard to tell from the outside when the answer is the same either way.
func logSearchDelegation(delegated bool, pattern string) {
	if delegated {
		vtui.DebugLog("EDITOR_SEARCH: %q answered by the file system", pattern)
		return
	}
	vtui.DebugLog("EDITOR_SEARCH: %q scanned locally", pattern)
}
