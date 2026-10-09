package textdiff

import (
	"reflect"
	"strings"
	"testing"
)

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// applyOps replays an edit script against a and checks it reproduces b
// exactly. This is the property that actually matters for a diff algorithm:
// whatever path it picks through the edit graph, applying it must round-trip.
func applyOps(t *testing.T, a, b []string, ops []Op) {
	t.Helper()
	var got []string
	ai := 0
	for _, op := range ops {
		switch op.Kind {
		case OpEqual:
			if op.ALine != ai {
				t.Fatalf("OpEqual out of order: ALine=%d, expected %d", op.ALine, ai)
			}
			got = append(got, a[op.ALine])
			ai++
		case OpDelete:
			if op.ALine != ai {
				t.Fatalf("OpDelete out of order: ALine=%d, expected %d", op.ALine, ai)
			}
			ai++
		case OpInsert:
			got = append(got, b[op.BLine])
		default:
			t.Fatalf("unknown op kind %v", op.Kind)
		}
	}
	if ai != len(a) {
		t.Fatalf("did not consume all of a: consumed %d of %d", ai, len(a))
	}
	if !reflect.DeepEqual(got, b) {
		t.Fatalf("replaying ops did not reproduce b:\n got:  %#v\n want: %#v", got, b)
	}
}

func TestDiffRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		a, b string
	}{
		{"both empty", "", ""},
		{"identical", "a\nb\nc", "a\nb\nc"},
		{"a empty", "", "x\ny"},
		{"b empty", "x\ny", ""},
		{"single line replace", "a", "b"},
		{"append", "a\nb", "a\nb\nc"},
		{"prepend", "b\nc", "a\nb\nc"},
		{"middle insert", "a\nc", "a\nb\nc"},
		{"middle delete", "a\nb\nc", "a\nc"},
		{"middle replace", "a\nb\nc", "a\nx\nc"},
		{"totally different", "a\nb\nc", "x\ny\nz"},
		{"repeated lines", "a\na\na", "a\na"},
		{"reorder-ish", "a\nb\nc\nd", "d\nc\nb\na"},
		{"trailing blank line", "a\nb\n", "a\nb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, b := lines(tc.a), lines(tc.b)
			ops, err := Diff(a, b)
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			applyOps(t, a, b, ops)
		})
	}
}

func TestDiffIdenticalIsAllEqual(t *testing.T) {
	a := []string{"one", "two", "three"}
	ops, err := Diff(a, a)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(ops) != 3 {
		t.Fatalf("expected 3 ops, got %d: %#v", len(ops), ops)
	}
	for i, op := range ops {
		if op.Kind != OpEqual || op.ALine != i || op.BLine != i {
			t.Fatalf("op %d: expected Equal(%d,%d), got %#v", i, i, i, op)
		}
	}
}

func TestDiffBothEmptyIsNoOps(t *testing.T) {
	ops, err := Diff(nil, nil)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(ops) != 0 {
		t.Fatalf("expected no ops for two empty inputs, got %#v", ops)
	}
}

func TestDiffTooLarge(t *testing.T) {
	a := make([]string, MaxLines)
	for i := range a {
		a[i] = "x"
	}
	b := []string{"y"}
	if _, err := Diff(a, b); err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestRowsSingleLineReplaceIsOnePairedRow(t *testing.T) {
	a := []string{"a"}
	b := []string{"b"}
	ops, err := Diff(a, b)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	rows := Rows(a, b, ops)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %#v", len(rows), rows)
	}
	r := rows[0]
	if r.Left.Kind != RowChanged || r.Left.Text != "a" {
		t.Fatalf("left side: got %#v", r.Left)
	}
	if r.Right.Kind != RowChanged || r.Right.Text != "b" {
		t.Fatalf("right side: got %#v", r.Right)
	}
}

func TestRowsPureInsertHasLeftFiller(t *testing.T) {
	a := []string(nil)
	b := []string{"x"}
	ops, err := Diff(a, b)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	rows := Rows(a, b, ops)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %#v", len(rows), rows)
	}
	if rows[0].Left.Kind != RowFiller {
		t.Fatalf("left side: expected RowFiller, got %#v", rows[0].Left)
	}
	if rows[0].Right.Kind != RowInserted || rows[0].Right.Text != "x" {
		t.Fatalf("right side: got %#v", rows[0].Right)
	}
}

func TestRowsPureDeleteHasRightFiller(t *testing.T) {
	a := []string{"x"}
	b := []string(nil)
	ops, err := Diff(a, b)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	rows := Rows(a, b, ops)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d: %#v", len(rows), rows)
	}
	if rows[0].Right.Kind != RowFiller {
		t.Fatalf("right side: expected RowFiller, got %#v", rows[0].Right)
	}
	if rows[0].Left.Kind != RowDeleted || rows[0].Left.Text != "x" {
		t.Fatalf("left side: got %#v", rows[0].Left)
	}
}

func TestRowsUnevenReplaceKeepsRemainderAsInsertOrDelete(t *testing.T) {
	// One line replaced by three: one paired "changed" row, then two pure
	// inserts with a filler on the left.
	a := []string{"a", "mid", "c"}
	b := []string{"a", "x", "y", "z", "c"}
	ops, err := Diff(a, b)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	rows := Rows(a, b, ops)
	applyOps(t, a, b, ops)

	var gotKinds []RowKind
	for _, r := range rows {
		gotKinds = append(gotKinds, r.Left.Kind)
	}
	// First and last row must be the unchanged "a" and "c" anchors.
	if rows[0].Left.Kind != RowEqual || rows[0].Left.Text != "a" {
		t.Fatalf("first row: got %#v", rows[0])
	}
	if rows[len(rows)-1].Left.Kind != RowEqual || rows[len(rows)-1].Left.Text != "c" {
		t.Fatalf("last row: got %#v", rows[len(rows)-1])
	}
	// Exactly one paired "changed" row and two left-filler rows in between.
	var changed, fillers int
	for _, r := range rows[1 : len(rows)-1] {
		switch {
		case r.Left.Kind == RowChanged && r.Right.Kind == RowChanged:
			changed++
		case r.Left.Kind == RowFiller && r.Right.Kind == RowInserted:
			fillers++
		default:
			t.Fatalf("unexpected middle row: %#v", r)
		}
	}
	if changed != 1 || fillers != 2 {
		t.Fatalf("expected 1 changed + 2 left-filler rows, got changed=%d fillers=%d (kinds=%v)", changed, fillers, gotKinds)
	}
}

func TestRowsCoverEveryInputLineExactlyOnce(t *testing.T) {
	a := []string{"1", "2", "3", "4", "5"}
	b := []string{"1", "x", "3", "y", "z", "5"}
	ops, err := Diff(a, b)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	rows := Rows(a, b, ops)

	seenA := make([]bool, len(a))
	seenB := make([]bool, len(b))
	for _, r := range rows {
		if r.Left.Kind != RowFiller {
			if seenA[r.Left.Line] {
				t.Fatalf("left line %d covered twice", r.Left.Line)
			}
			seenA[r.Left.Line] = true
		}
		if r.Right.Kind != RowFiller {
			if seenB[r.Right.Line] {
				t.Fatalf("right line %d covered twice", r.Right.Line)
			}
			seenB[r.Right.Line] = true
		}
	}
	for i, ok := range seenA {
		if !ok {
			t.Fatalf("left line %d never appeared in any row", i)
		}
	}
	for i, ok := range seenB {
		if !ok {
			t.Fatalf("right line %d never appeared in any row", i)
		}
	}
}
