package fileops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"io"
	"sort"
	"strings"
	"time"
)

// Folder comparison, as Far3 offers it in two places at once: the built-in
// "Compare folders" command, which marks what one panel has and the other
// does not, and the Advanced Compare dialog, which adds recursion, a
// marked-only scope and a real content comparison. f4 has a single command
// carrying the options of the second one, with the first one's behaviour
// (name plus time and size) as the default.
//
// Everything in this file is deliberately free of UI: the dialog and the
// progress reporting live in compare_folders_ui.go, so the comparison
// itself can be tested against a temporary directory.

const (
	// compareTimeSlack is Far's "two-second precision": FAT stores the
	// modification time in two-second steps, so a file copied from one is
	// routinely a second away from its source.
	compareTimeSlack = 2 * time.Second
	// compareZoneStep is the granularity of time zone offsets. Every zone
	// in use is a whole number of quarter hours away from UTC, so a
	// difference that is a multiple of one may be a zone artifact rather
	// than a real edit.
	compareZoneStep = 15 * time.Minute
	// compareZoneSpan bounds that rule. The inhabited zones run from
	// UTC-12 to UTC+14, so nothing further apart than their spread can be
	// explained by a zone — and without the bound, two files a year apart
	// to the second would pass as equal.
	compareZoneSpan = 26 * time.Hour
	// compareChunkSize is how much of a file is read at a time when
	// comparing contents.
	compareChunkSize = 64 * 1024
)

// CompareItem is one file or folder found below a panel's folder.
type CompareItem struct {
	// rel is the path relative to the panel folder, always slash
	// separated so both sides compare as strings regardless of the file
	// system they came from.
	rel string
	// top is the first component of rel, i.e. the panel entry that gets
	// marked when this item turns out to differ.
	top string
	// full is the path in the item's own file system.
	full string
	item vfs.VFSItem
}

// CompareOutcome is what a comparison marks, and what it looked at.
type CompareOutcome struct {
	// Left and Right hold panel entry names, not relative paths: a
	// difference three folders down marks the top-level folder the user
	// can actually see, the way Far does.
	Left  map[string]bool
	Right map[string]bool
	// compared counts the pairs examined, Differing the pairs (and lone
	// items) that turned out not to match.
	compared  int
	Differing int
	// ReadErr is the first file that could not be read. Its pair is
	// treated as differing — an unreadable file is not a match — but the
	// comparison carries on and the error is reported once at the end.
	ReadErr error
}

func newCompareOutcome() *CompareOutcome {
	return &CompareOutcome{Left: make(map[string]bool), Right: make(map[string]bool)}
}

// markLeft and markRight record the panel entry to select.
func (o *CompareOutcome) markLeft(item CompareItem)  { o.Left[item.top] = true }
func (o *CompareOutcome) markRight(item CompareItem) { o.Right[item.top] = true }

// compareProgress is called while the tree is walked and while pairs are
// compared, so the dialog can show where the work currently is.
type compareProgress func(path string, done, total int)

// compareTimesEqual answers whether two modification times count as the
// same one under the current options.
func compareTimesEqual(a, b time.Time, opts config.CompareOptions) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	slack := time.Duration(0)
	if opts.TimeSlack {
		slack = compareTimeSlack
	}
	if d <= slack {
		return true
	}
	if opts.IgnoreZones && d <= compareZoneSpan+slack {
		// A zone difference is a multiple of a quarter of an hour; the
		// slack applies around that multiple, not only around zero.
		rest := d % compareZoneStep
		if rest <= slack || compareZoneStep-rest <= slack {
			return true
		}
	}
	return false
}

// compareMetadata compares two files by everything that can be answered
// from the directory listing alone. It reports whether the metadata
// differs, whether that difference is one of time only, and which side is
// the newer one (1 left, -1 right, 0 neither).
func compareMetadata(a, b vfs.VFSItem, opts config.CompareOptions) (differs, timeOnly bool, newer int) {
	sizeDiffers := opts.BySize && a.Size != b.Size
	timeDiffers := opts.ByTime && !compareTimesEqual(a.MTime, b.MTime, opts)
	if timeDiffers {
		switch {
		case a.MTime.After(b.MTime):
			newer = 1
		case b.MTime.After(a.MTime):
			newer = -1
		}
	}
	return sizeDiffers || timeDiffers, timeDiffers && !sizeDiffers, newer
}

// compareSizeKnown reports whether a listing gave a usable length. A
// remote object may not know its own size until it is opened, and VFSItem
// documents non-zero Size as always known.
func compareSizeKnown(item vfs.VFSItem) bool {
	return item.SizeKnown || item.Size != 0
}

// firstPathComponent returns the panel entry a relative path lives under.
func firstPathComponent(rel string) string {
	if i := strings.IndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return rel
}

// CollectCompareSide walks one panel's folder and returns everything worth
// comparing, keyed by relative path. allow, when not nil, restricts the
// walk to those top-level names — this is the "only marked" scope.
//
// A subfolder that cannot be read is skipped rather than aborting the
// whole comparison: one unreadable folder should not cost the answer about
// every other one. An unreadable panel folder is fatal, because then there
// is nothing to compare at all.
func CollectCompareSide(ctx context.Context, v vfs.VFS, root string, allow map[string]bool, opts config.CompareOptions, progress func(string)) (map[string]CompareItem, error) {
	if v == nil {
		return nil, errors.New("compare: no file system")
	}
	items := make(map[string]CompareItem)
	type frame struct {
		full  string
		rel   string
		depth int
	}
	stack := []frame{{full: root}}
	for len(stack) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if progress != nil {
			progress(cur.rel)
		}

		var children []vfs.VFSItem
		err := v.ReadDir(ctx, cur.full, func(chunk []vfs.VFSItem) {
			children = append(children, chunk...)
		})
		if err != nil {
			if cur.depth == 0 {
				return nil, err
			}
			continue
		}

		for _, child := range children {
			if child.Name == "." || child.Name == ".." || child.Name == "" {
				continue
			}
			if cur.depth == 0 && allow != nil && !allow[child.Name] {
				continue
			}
			if child.IsDir && !opts.Recursive {
				// Far's built-in comparison is about files. Without
				// recursion a folder carries no information beyond its
				// name, and marking it would promise a comparison that
				// did not happen.
				continue
			}
			rel := child.Name
			top := child.Name
			if cur.rel != "" {
				rel = cur.rel + "/" + child.Name
				top = firstPathComponent(cur.rel)
			}
			full := v.Join(cur.full, child.Name)
			items[rel] = CompareItem{rel: rel, top: top, full: full, item: child}

			if !child.IsDir || child.IsSymlink {
				// A symlinked folder is a leaf: following it invites a
				// loop, and its target is compared where it really lives.
				continue
			}
			if opts.LimitDepth && cur.depth+1 > opts.MaxDepth {
				continue
			}
			stack = append(stack, frame{full: full, rel: rel, depth: cur.depth + 1})
		}
	}
	return items, nil
}

// CompareSides is the comparison proper: it pairs the two collections by
// relative path and decides, for every pair, which side to mark.
func CompareSides(ctx context.Context, leftFS, rightFS vfs.VFS, left, right map[string]CompareItem, opts config.CompareOptions, progress compareProgress) (*CompareOutcome, error) {
	out := newCompareOutcome()

	keys := make([]string, 0, len(left)+len(right))
	for rel := range left {
		keys = append(keys, rel)
	}
	for rel := range right {
		if _, both := left[rel]; !both {
			keys = append(keys, rel)
		}
	}
	sort.Strings(keys)

	skip := compareSkipMode(opts)
	for i, rel := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if progress != nil {
			progress(rel, i, len(keys))
		}

		l, hasLeft := left[rel]
		r, hasRight := right[rel]
		switch {
		case !hasRight:
			out.Differing++
			out.markLeft(l)
			continue
		case !hasLeft:
			out.Differing++
			out.markRight(r)
			continue
		}

		if l.item.IsDir && r.item.IsDir {
			// Two folders of the same name are the container of the
			// comparison, not a subject of it.
			continue
		}
		out.compared++
		if l.item.IsDir != r.item.IsDir {
			// A file on one side and a folder on the other is the
			// starkest difference there is.
			out.Differing++
			out.markLeft(l)
			out.markRight(r)
			continue
		}

		differs, timeOnly, newer := compareMetadata(l.item, r.item, opts)
		if opts.ByContent && !differs {
			equal, err := compareContents(ctx, leftFS, l, rightFS, r, skip)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				if out.ReadErr == nil {
					out.ReadErr = err
				}
				equal = false
			}
			if !equal {
				differs, timeOnly = true, false
			}
		}
		if !differs {
			continue
		}

		out.Differing++
		// Far marks the newer file when the only difference is the time:
		// that is the one worth copying over. Any other difference says
		// nothing about direction, so both sides are marked.
		switch {
		case timeOnly && newer > 0:
			out.markLeft(l)
		case timeOnly && newer < 0:
			out.markRight(r)
		default:
			out.markLeft(l)
			out.markRight(r)
		}
	}
	return out, nil
}

// compareSkipMode turns the ignore options into the content filter mode,
// where -1 means "compare the bytes as they are".
func compareSkipMode(opts config.CompareOptions) int {
	if !opts.Ignore {
		return -1
	}
	if opts.IgnoreMode == config.CompareIgnoreSpaces {
		return config.CompareIgnoreSpaces
	}
	return config.CompareIgnoreEOL
}

// compareContents answers whether two files hold the same bytes, under the
// filter the options asked for.
func compareContents(ctx context.Context, leftFS vfs.VFS, left CompareItem, rightFS vfs.VFS, right CompareItem, skip int) (bool, error) {
	if skip < 0 && compareSizeKnown(left.item) && compareSizeKnown(right.item) && left.item.Size != right.item.Size {
		// Different lengths cannot hold the same bytes, and not opening
		// the files at all is the whole point of asking first.
		return false, nil
	}

	lf, err := leftFS.Open(ctx, left.full)
	if err != nil {
		return false, fmt.Errorf("%s: %w", left.full, err)
	}
	defer lf.Close()
	rf, err := rightFS.Open(ctx, right.full)
	if err != nil {
		return false, fmt.Errorf("%s: %w", right.full, err)
	}
	defer rf.Close()

	return compareStreams(newCompareStream(ctx, lf, skip), newCompareStream(ctx, rf, skip))
}

// compareStream reads a file in chunks and hands out the filtered bytes.
type compareStream struct {
	ctx  context.Context
	File vfs.ReadAtCloser
	// off is tracked here rather than relying on the sequential Read:
	// ReadAt is the call every VFS implements for the viewer, so it is
	// the one that can be relied on.
	off  int64
	skip int
	raw  []byte
	work []byte
	// buf is what has been read and filtered but not yet compared.
	buf []byte
	// pendingCR remembers a carriage return at the very end of a chunk,
	// so a CRLF split across two reads still collapses into one break.
	pendingCR bool
	eof       bool
}

func newCompareStream(ctx context.Context, File vfs.ReadAtCloser, skip int) *compareStream {
	return &compareStream{ctx: ctx, File: File, skip: skip, raw: make([]byte, compareChunkSize)}
}

// fill makes sure buf holds at least one byte, unless the file is over.
func (s *compareStream) fill() error {
	for len(s.buf) == 0 && !s.eof {
		if err := s.ctx.Err(); err != nil {
			return err
		}
		n, err := s.File.ReadAt(s.ctx, s.raw, s.off)
		if n > 0 {
			s.off += int64(n)
			s.buf = s.normalize(s.raw[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.eof = true
				continue
			}
			return err
		}
		if n == 0 {
			s.eof = true
		}
	}
	return nil
}

// normalize applies the ignore filter to one chunk. Without a filter the
// chunk is handed on as it is, so the common case copies nothing.
func (s *compareStream) normalize(chunk []byte) []byte {
	switch s.skip {
	case config.CompareIgnoreSpaces:
		out := s.work[:0]
		for _, b := range chunk {
			switch b {
			case ' ', '\t', '\r', '\n', '\v', '\f':
			default:
				out = append(out, b)
			}
		}
		s.work = out
		return out
	case config.CompareIgnoreEOL:
		out := s.work[:0]
		for _, b := range chunk {
			switch b {
			case '\r':
				out = append(out, '\n')
				s.pendingCR = true
			case '\n':
				if s.pendingCR {
					s.pendingCR = false
					continue
				}
				out = append(out, '\n')
			default:
				s.pendingCR = false
				out = append(out, b)
			}
		}
		s.work = out
		return out
	default:
		return chunk
	}
}

// compareStreams walks both files side by side and stops at the first
// difference.
func compareStreams(a, b *compareStream) (bool, error) {
	for {
		if err := a.fill(); err != nil {
			return false, err
		}
		if err := b.fill(); err != nil {
			return false, err
		}
		if len(a.buf) == 0 || len(b.buf) == 0 {
			// One of them ran out: they match only if both did.
			return len(a.buf) == 0 && len(b.buf) == 0, nil
		}
		n := len(a.buf)
		if len(b.buf) < n {
			n = len(b.buf)
		}
		if !bytes.Equal(a.buf[:n], b.buf[:n]) {
			return false, nil
		}
		a.buf = a.buf[n:]
		b.buf = b.buf[n:]
	}
}
