package main

import (
	"bytes"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
)

// applyPieces is what a remote host does with the pieces, done locally so
// the description can be checked against the text it is supposed to mean.
func applyPieces(original []byte, pieces []vfs.PatchPiece) []byte {
	var out []byte
	for _, p := range pieces {
		if p.Data != nil {
			out = append(out, p.Data...)
			continue
		}
		out = append(out, original[p.Offset:p.Offset+p.Length]...)
	}
	return out
}

func TestPatchPiecesFromTable(t *testing.T) {
	original := []byte("the quick brown fox jumps over the lazy dog")

	cases := []struct {
		name string
		edit func(pt *piecetable.PieceTable)
	}{
		{"untouched", func(pt *piecetable.PieceTable) {}},
		{"one byte inserted", func(pt *piecetable.PieceTable) { pt.Insert(20, []byte("X")) }},
		{"one byte replaced", func(pt *piecetable.PieceTable) {
			pt.Delete(4, 1)
			pt.Insert(4, []byte("Q"))
		}},
		{"head deleted", func(pt *piecetable.PieceTable) { pt.Delete(0, 4) }},
		{"tail deleted", func(pt *piecetable.PieceTable) { pt.Delete(39, 4) }},
		{"appended", func(pt *piecetable.PieceTable) { pt.Insert(43, []byte(" and back")) }},
		{"emptied", func(pt *piecetable.PieceTable) { pt.Delete(0, 43) }},
		{"rewritten", func(pt *piecetable.PieceTable) {
			pt.Delete(0, 43)
			pt.Insert(0, []byte("nothing of the original is left"))
		}},
	}

	for _, tc := range cases {
		pt := piecetable.New(append([]byte(nil), original...))
		tc.edit(pt)

		pieces, ok := patchPiecesFromTable(pt)
		if !ok {
			t.Fatalf("%s: the table could not be described", tc.name)
		}

		want, err := pt.Bytes()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := applyPieces(original, pieces); !bytes.Equal(got, want) {
			t.Errorf("%s: pieces produce %q, want %q", tc.name, got, want)
		}

		// The whole point is that the untouched parts are described rather
		// than sent, so a small edit must not turn into a large literal.
		var literal int64
		for _, p := range pieces {
			if p.Data != nil {
				literal += int64(len(p.Data))
			}
		}
		if tc.name == "one byte inserted" && literal != 1 {
			t.Errorf("inserting one byte would send %d bytes", literal)
		}
		if tc.name == "untouched" && literal != 0 {
			t.Errorf("an untouched file would send %d bytes", literal)
		}
	}
}
