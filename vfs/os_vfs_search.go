package vfs

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/unxed/f4/vfs/hostfs"
	"github.com/unxed/f4/vfs/hostpath"
)

// FindFiles is the local fast path for Find File. ReadDir is deliberately a
// rich panel-listing API: it stats every entry so a panel can render size,
// timestamps, permissions, and physical size. A search only needs names and
// directory bits for rejected entries, so doing that work for every item made
// a large local tree pay one stat per entry before the actual match decision.
// This walk keeps metadata lazy and stats only hits, which is the same shape
// as the native find implementations used by Far.
func (v *OSVFS) FindFiles(ctx context.Context, dir string, q FindQuery) ([]FoundEntry, error) {
	masks := q.Masks
	if len(masks) == 0 {
		masks = []string{"*"}
	}
	matcher, err := newFindQueryMatcher(q)
	if err != nil {
		return nil, err
	}

	var found []FoundEntry
	var scanned int64
	var lastProgress time.Time
	report := func(path string, force bool) {
		if q.Progress == nil {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastProgress) < 100*time.Millisecond {
			return
		}
		lastProgress = now
		q.Progress(FindProgress{Scanned: scanned, Found: int64(len(found)), Path: path})
	}

	var walk func(string) error
	walk = func(current string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		report(current, false)
		entries, err := hostfs.ReadDir(prepareOSPath(current))
		if err != nil {
			// The root error is useful to the caller. A child that becomes
			// inaccessible during a long search is treated like Far's scan:
			// skip it and continue with the rest of the tree.
			if current == dir {
				return err
			}
			return nil
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if q.Limit > 0 && len(found) >= q.Limit {
				return nil
			}
			name := entry.Name()
			if name == "." || name == ".." {
				continue
			}
			scanned++
			child := hostpath.Join(current, name)
			isSymlink := entry.Type()&os.ModeSymlink != 0
			isDir := entry.IsDir()
			if isSymlink {
				if !q.FindSymlinks {
					continue
				}
				// Symlinks are leaves even when their target is a directory.
				// Following them is both surprising in a search and a common
				// source of recursive loops in home directories.
				isDir = false
			}

			if isDir {
				if q.FindFolders && q.Text == "" && findMaskMatches(name, masks) {
					if item, statErr := v.Stat(ctx, child); statErr == nil {
						found = append(found, FoundEntry{Path: child, Item: item})
						report(child, true)
					}
				}
				if err := walk(child); err != nil {
					return err
				}
				continue
			}

			if !findMaskMatches(name, masks) {
				continue
			}
			if matcher != nil {
				contains, matchErr := v.findFileContains(ctx, child, matcher)
				if matchErr != nil || !contains {
					continue
				}
			}
			item, statErr := v.Stat(ctx, child)
			if statErr != nil {
				continue
			}
			found = append(found, FoundEntry{Path: child, Item: item})
			report(child, true)
		}
		return nil
	}

	if err := walk(dir); err != nil {
		return found, err
	}
	report(dir, true)
	return found, nil
}

func findMaskMatches(name string, masks []string) bool {
	for _, mask := range masks {
		if mask == "" {
			continue
		}
		if matched, _ := filepath.Match(mask, name); matched {
			return true
		}
	}
	return false
}

type findQueryMatcher struct {
	query  FindQuery
	needle string
	folded string
	regex  *regexp.Regexp
}

func newFindQueryMatcher(q FindQuery) (*findQueryMatcher, error) {
	if q.Text == "" {
		return nil, nil
	}
	m := &findQueryMatcher{query: q, needle: q.Text}
	if q.IgnoreCase {
		m.folded = strings.ToLower(q.Text)
	}
	if q.Regex {
		pattern := q.Text
		if q.IgnoreCase {
			pattern = "(?i:" + pattern + ")"
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		m.regex = re
	}
	return m, nil
}

func (m *findQueryMatcher) hasMatch(data []byte) bool {
	if m == nil {
		return false
	}
	s := string(data)
	if m.regex != nil {
		for _, indexes := range m.regex.FindAllStringIndex(s, -1) {
			if !m.query.WholeWords || findWholeWord(s, indexes[0], indexes[1]) {
				return true
			}
		}
		return false
	}
	needle := m.needle
	haystack := s
	if m.query.IgnoreCase {
		needle = m.folded
		haystack = strings.ToLower(s)
	}
	for from := 0; from <= len(haystack); {
		at := strings.Index(haystack[from:], needle)
		if at < 0 {
			break
		}
		at += from
		end := at + len(needle)
		if !m.query.WholeWords || findWholeWord(s, at, end) {
			return true
		}
		from = at + 1
	}
	return false
}

func findWholeWord(s string, start, end int) bool {
	return !findWordBefore(s, start) && !findWordAfter(s, end)
}

func findWordBefore(s string, at int) bool {
	if at <= 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:at])
	return isFindWordRune(r)
}

func findWordAfter(s string, at int) bool {
	if at >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[at:])
	return isFindWordRune(r)
}

func isFindWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (v *OSVFS) findFileContains(ctx context.Context, path string, matcher *findQueryMatcher) (bool, error) {
	f, err := v.Open(ctx, path)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = f.Close() // Search opens the handle only for reading.
	}()

	overlap := len(matcher.needle) + 4
	if matcher.regex != nil && overlap < 4096 {
		overlap = 4096
	}
	if overlap < 1 {
		overlap = 1
	}
	buf := make([]byte, 128*1024)
	var carry []byte
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		n, readErr := f.Read(ctx, buf)
		if n > 0 {
			data := make([]byte, 0, len(carry)+n)
			data = append(data, carry...)
			data = append(data, buf[:n]...)
			if matcher.hasMatch(data) {
				return !matcher.query.NotContaining, nil
			}
			if len(data) > overlap {
				carry = append(carry[:0], data[len(data)-overlap:]...)
			} else {
				carry = append(carry[:0], data...)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return matcher.query.NotContaining, nil
			}
			return false, readErr
		}
	}
}
