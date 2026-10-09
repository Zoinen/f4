// Package textdiff computes a line-level diff between two texts, the way
// Total Commander's "Compare by content" does it: a shortest edit script
// (Myers, "An O(ND) Difference Algorithm", 1986) turned into a list of rows
// suitable for a side-by-side view, where a pure insertion or deletion gets a
// blank filler on the other side and a same-position replacement is paired
// line-by-line up to the shorter of the two runs.
//
// This is deliberately independent of vtui: the algorithm is plain data in,
// plain data out, so it can be tested without a screen.
package textdiff

import "errors"

// OpKind is what a single edit-script step does to line a (left) or line b
// (right).
type OpKind int

const (
	// OpEqual means the same line (by content) appears in both texts.
	OpEqual OpKind = iota
	// OpDelete means the line exists only in the left text.
	OpDelete
	// OpInsert means the line exists only in the right text.
	OpInsert
)

// Op is one step of the edit script, in left-to-right (top-to-bottom) order.
// ALine/BLine are 0-based indices into the input slices; the side an op does
// not touch is -1.
type Op struct {
	Kind         OpKind
	ALine, BLine int
}

// ErrTooLarge is returned by Diff when a and b together exceed MaxLines.
// Myers' algorithm is O((N+M)*D) time and O(D^2) space, where D is the
// number of differing lines: cheap when the two texts are mostly alike (the
// common case for "compare two versions of a file"), but its worst case
// (every line different) is quadratic in the input size. MaxLines bounds
// that worst case for a first, synchronous implementation; a background
// task and a higher or configurable limit are natural follow-ups, not
// something a basic viewer needs on day one.
var ErrTooLarge = errors.New("textdiff: input too large for a content comparison")

// MaxLines is the largest combined line count (len(a)+len(b)) Diff accepts.
// 20000 combined lines caps the worst-case trace storage at a few tens of
// megabytes of ints, comfortably inside what a foreground UI action may
// allocate without a progress dialog.
const MaxLines = 20000

// Diff returns the shortest edit script turning a into b, as a sequence of
// OpEqual/OpDelete/OpInsert steps. It returns ErrTooLarge instead of running
// when len(a)+len(b) > MaxLines.
func Diff(a, b []string) ([]Op, error) {
	n, m := len(a), len(b)
	if n+m > MaxLines {
		return nil, ErrTooLarge
	}
	if n == 0 && m == 0 {
		return nil, nil
	}

	max := n + m
	offset := max
	// v[offset+k] is the largest x reached on diagonal k = x-y using the
	// fewest edits found so far. trace[d] is a snapshot of v taken *before*
	// depth d runs, i.e. it holds the depth-(d-1) values the depth-d step
	// reads from (a step at depth d only ever reads k-1/k+1, both of depth
	// d-1's parity, and only ever writes entries of depth d's own parity —
	// so nothing depth d writes is visible to depth d itself).
	v := make([]int, 2*max+1)
	trace := make([][]int, 0, max+1)

	d := 0
	found := false
loop:
	for ; d <= max; d++ {
		snapshot := make([]int, len(v))
		copy(snapshot, v)
		trace = append(trace, snapshot)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1] // came from an insertion (down move)
			} else {
				x = v[offset+k-1] + 1 // came from a deletion (right move)
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				found = true
				break loop
			}
		}
	}
	if !found {
		// Every possible (x,y) pair was exhausted without reaching (n,m).
		// Diff's own bookkeeping guarantees this cannot happen; treat it as
		// the trivial "everything differs" case rather than panicking on a
		// slice index below.
		d = max
	}

	// Backtrack from (n,m) to (0,0), depth by depth, emitting ops in reverse
	// (right-to-left / bottom-to-top) order.
	ops := make([]Op, 0, d+min(n, m))
	x, y := n, m
	for depth := d; depth > 0; depth-- {
		vv := trace[depth]
		k := x - y
		var prevK int
		if k == -depth || (k != depth && vv[offset+k-1] < vv[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := vv[offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x--
			y--
			ops = append(ops, Op{OpEqual, x, y})
		}
		if x == prevX {
			y--
			ops = append(ops, Op{OpInsert, -1, y})
		} else {
			x--
			ops = append(ops, Op{OpDelete, x, -1})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		ops = append(ops, Op{OpEqual, x, y})
	}

	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops, nil
}

// RowKind describes how one side of a Row relates to the other.
type RowKind int

const (
	// RowEqual is an unchanged line, identical on both sides.
	RowEqual RowKind = iota
	// RowChanged is a same-position replacement: both sides have text, and
	// it differs.
	RowChanged
	// RowDeleted marks a line that exists only on the left side; the right
	// side of the same row is a RowFiller.
	RowDeleted
	// RowInserted marks a line that exists only on the right side; the left
	// side of the same row is a RowFiller.
	RowInserted
	// RowFiller marks a side that has no line at this row because the other
	// side had a pure insertion or deletion here.
	RowFiller
)

// Side is one half of a Row.
type Side struct {
	Kind RowKind
	// Line is the 0-based index into the original slice this side came
	// from, or -1 for RowFiller.
	Line int
	Text string
}

// Row is one line of a side-by-side view: at most one of Left/Right is a
// RowFiller at a time, since a genuine same-position change (RowChanged)
// carries text on both sides.
type Row struct {
	Left, Right Side
}

// Rows turns an edit script into side-by-side rows: a run of deletions
// followed by a run of insertions (the usual shape of a "replace" edit) is
// paired line-by-line up to the shorter run's length, and any remainder
// keeps its own row with a filler on the other side. A lone deletion or
// insertion (no matching run of the opposite kind next to it) is a row with
// a filler on the other side, too.
func Rows(a, b []string, ops []Op) []Row {
	rows := make([]Row, 0, len(ops))
	i := 0
	for i < len(ops) {
		switch ops[i].Kind {
		case OpEqual:
			op := ops[i]
			rows = append(rows, Row{
				Left:  Side{Kind: RowEqual, Line: op.ALine, Text: a[op.ALine]},
				Right: Side{Kind: RowEqual, Line: op.BLine, Text: b[op.BLine]},
			})
			i++
		case OpDelete, OpInsert:
			// Collect the contiguous run of deletes and, immediately after
			// it, the contiguous run of inserts (either run may be empty on
			// its own, but at least one of them is this op).
			delStart := i
			for i < len(ops) && ops[i].Kind == OpDelete {
				i++
			}
			delEnd := i
			insStart := i
			for i < len(ops) && ops[i].Kind == OpInsert {
				i++
			}
			insEnd := i

			nDel := delEnd - delStart
			nIns := insEnd - insStart
			paired := min(nDel, nIns)
			for j := 0; j < paired; j++ {
				al := ops[delStart+j].ALine
				bl := ops[insStart+j].BLine
				rows = append(rows, Row{
					Left:  Side{Kind: RowChanged, Line: al, Text: a[al]},
					Right: Side{Kind: RowChanged, Line: bl, Text: b[bl]},
				})
			}
			for j := paired; j < nDel; j++ {
				al := ops[delStart+j].ALine
				rows = append(rows, Row{
					Left:  Side{Kind: RowDeleted, Line: al, Text: a[al]},
					Right: Side{Kind: RowFiller, Line: -1},
				})
			}
			for j := paired; j < nIns; j++ {
				bl := ops[insStart+j].BLine
				rows = append(rows, Row{
					Left:  Side{Kind: RowFiller, Line: -1},
					Right: Side{Kind: RowInserted, Line: bl, Text: b[bl]},
				})
			}
		}
	}
	return rows
}
