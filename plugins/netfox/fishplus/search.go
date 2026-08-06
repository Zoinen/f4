package fishplus

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DefaultGrepLimit caps how many matches one request brings back. A search
// across a large log can match millions of times, and nobody scrolls through
// millions of hits; the caller asks again with a narrower pattern instead.
const DefaultGrepLimit = 10000

// ErrNoGrep is returned when the remote host lacks the tools the search is
// built from. The caller is expected to fall back to reading the file.
var ErrNoGrep = errors.New("fishplus: the remote host cannot search files")

// GrepOptions selects how the pattern is interpreted and how much comes back.
type GrepOptions struct {
	// Fixed treats the pattern as a plain string rather than as an extended
	// regular expression.
	Fixed bool
	// IgnoreCase folds case the way the remote grep does it, which for a
	// non-ASCII pattern is the remote locale's idea of case.
	IgnoreCase bool
	// Limit caps the number of matches; zero means DefaultGrepLimit.
	Limit int
}

func (o GrepOptions) mode() string {
	m := "e"
	if o.Fixed {
		m = "f"
	}
	if o.IgnoreCase {
		m += "i"
	}
	return m
}

// CanGrep reports whether the remote host announced the tools the search
// needs. A host without them is not an error, it is a fallback to reading.
func (c *Client) CanGrep() bool {
	feats := c.sess.Features()
	return feats.Has("grep") && feats.Has("awk")
}

// Grep runs the search on the remote host and returns the byte offset of
// every match, in the order the file has them. Only the offsets travel: the
// matched text stays where it is, which is the whole reason for searching
// remotely instead of downloading the file.
func (c *Client) Grep(ctx context.Context, p, pattern string, opts GrepOptions) ([]int64, error) {
	if pattern == "" {
		return nil, errors.New("fishplus: empty search pattern")
	}
	if !c.CanGrep() {
		return nil, ErrNoGrep
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultGrepLimit
	}
	resp, err := c.sess.ExecPaths(ctx, "grep", []string{pattern, p}, opts.mode(), strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	if err := resp.Err("grep " + p); err != nil {
		return nil, err
	}
	offsets := make([]int64, 0, len(resp.Lines))
	for _, line := range resp.Lines {
		if line == "" {
			continue
		}
		off, convErr := strconv.ParseInt(line, 10, 64)
		if convErr != nil {
			// A diagnostic line from a remote tool must not cost the caller
			// the matches that did parse.
			continue
		}
		offsets = append(offsets, off)
	}
	if len(offsets) > limit {
		return nil, fmt.Errorf("fishplus: grep %q returned %d offsets for a limit of %d", p, len(offsets), limit)
	}
	return offsets, nil
}

// MaxFindResults caps how many hits one tree search brings back. Nobody
// scrolls through more, and the remote host stops walking once it has them.
const MaxFindResults = 10000

// ErrNoFind is returned when the remote host has no listing backend at all,
// so it cannot describe what it found even if it found it.
var ErrNoFind = errors.New("fishplus: the remote host cannot search a tree")

// FindOptions describes a tree search: which names to match, and optionally
// what the file has to contain.
type FindOptions struct {
	// Masks are shell globs matched against the file name, as find's -name
	// does it. At least one is required.
	Masks []string
	// Text, when set, keeps only files containing it.
	Text string
	// Fixed treats Text as a plain string rather than a regular expression.
	Fixed bool
	// IgnoreCase folds case for Text.
	IgnoreCase bool
	// Limit caps the number of hits; zero means MaxFindResults.
	Limit int
}

// CanFind reports whether the remote host can walk a tree for us. Anything
// that can list a directory can, since the walking is find's job.
func (c *Client) CanFind() bool { return c.sess.Features().ListingMode() != "" }

// Find walks a whole tree on the remote host and returns one entry per hit,
// with the full path in Entry.Name. A content search runs as a remote grep
// inside the same find, so a candidate is never downloaded just to be
// rejected — which is the difference between searching a remote source tree
// and waiting for it to arrive.
func (c *Client) Find(ctx context.Context, dir string, opts FindOptions) ([]Entry, error) {
	masks := make([]string, 0, len(opts.Masks))
	for _, m := range opts.Masks {
		if m != "" {
			masks = append(masks, m)
		}
	}
	if len(masks) == 0 {
		return nil, errors.New("fishplus: find needs at least one mask")
	}
	if !c.CanFind() {
		return nil, ErrNoFind
	}
	if opts.Text != "" && !c.CanGrep() {
		return nil, ErrNoGrep
	}
	limit := opts.Limit
	if limit <= 0 || limit > MaxFindResults {
		limit = MaxFindResults
	}

	gmode := "-"
	paths := append([]string{dir}, masks...)
	if opts.Text != "" {
		gmode = GrepOptions{Fixed: opts.Fixed, IgnoreCase: opts.IgnoreCase}.mode()
		paths = append(paths, opts.Text)
	}

	resp, err := c.sess.ExecPaths(ctx, "ffind", paths,
		strconv.Itoa(limit), strconv.Itoa(len(masks)), gmode)
	if err != nil {
		return nil, err
	}
	if err := resp.Err("ffind " + dir); err != nil {
		return nil, err
	}
	_, entries, err := ParseFoundListing(resp.Lines)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// LineIndex is what the remote host knows about the line structure of a file
// after one pass over it: where the requested lines start, and how many
// there are in total.
type LineIndex struct {
	// First is the one-based number of the line Offsets[0] belongs to.
	First int64
	// Offsets holds the byte offset of each line start, in file order. It is
	// shorter than requested when the file ends first.
	Offsets []int64
	// Total is the number of lines in the file.
	Total int64
}

// MaxLineIndexCount caps one index request. The offsets come back as text,
// so asking for millions of them would cost more than reading the file.
const MaxLineIndexCount = 100000

// CanIndexLines reports whether the remote host can do the counting.
func (c *Client) CanIndexLines() bool { return c.sess.Features().Has("awk") }

// Lines returns where the given range of lines starts, counting from line 1,
// together with the number of lines in the file. The remote host walks the
// file; what crosses the network is a few numbers, which is what lets a
// viewer jump to the end of a multi-gigabyte log over ssh.
func (c *Client) Lines(ctx context.Context, p string, first, count int64) (LineIndex, error) {
	if first < 1 {
		return LineIndex{}, fmt.Errorf("fishplus: lines %q: first line %d is below 1", p, first)
	}
	if count < 0 || count > MaxLineIndexCount {
		return LineIndex{}, fmt.Errorf("fishplus: lines %q: count %d outside 0..%d", p, count, MaxLineIndexCount)
	}
	if !c.CanIndexLines() {
		return LineIndex{}, ErrNoGrep
	}
	resp, err := c.sess.ExecPath(ctx, "lidx", p,
		strconv.FormatInt(first, 10), strconv.FormatInt(count, 10))
	if err != nil {
		return LineIndex{}, err
	}
	if err := resp.Err("lidx " + p); err != nil {
		return LineIndex{}, err
	}
	idx := LineIndex{First: first, Total: -1}
	for _, line := range resp.Lines {
		if strings.HasPrefix(line, "T ") {
			total, convErr := strconv.ParseInt(strings.TrimSpace(line[2:]), 10, 64)
			if convErr != nil {
				return LineIndex{}, fmt.Errorf("fishplus: bad line total %q", line)
			}
			idx.Total = total
			continue
		}
		off, convErr := strconv.ParseInt(line, 10, 64)
		if convErr != nil {
			continue
		}
		idx.Offsets = append(idx.Offsets, off)
	}
	if idx.Total < 0 {
		return LineIndex{}, errors.New("fishplus: line index without a total")
	}
	return idx, nil
}
