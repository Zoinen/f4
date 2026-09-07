package piecetable

// BufferType indicates which buffer a text fragment is in.
import (
	"errors"
	"sync"
)

var ErrLoading = errors.New("data is loading")

// BufferType indicates which buffer a text fragment is in.
type Buffer interface {
	Size() int
	Read(offset, length int) ([]byte, error)
}

// Viewer is an optional Buffer capability: a buffer that can hand out a
// window into its own bytes instead of a copy of them. A buffer answers
// false whenever it cannot — the range is not contiguous in its memory, or
// not resident yet — and the caller falls back to Read.
//
// The window aliases the buffer, so callers must treat it as read-only. That
// is safe for the Original buffer of a piece table by construction: edits
// only ever append to the add buffer, and no code path writes through an
// Original buffer.
type Viewer interface {
	View(offset, length int) ([]byte, bool)
}

type MemoryBuffer []byte

func (m MemoryBuffer) Size() int { return len(m) }
func (m MemoryBuffer) Read(offset, length int) ([]byte, error) {
	if offset < 0 || offset >= len(m) || length <= 0 {
		return nil, nil
	}
	end := offset + length
	if end > len(m) {
		end = len(m)
	}
	return m[offset:end], nil
}

// View implements Viewer. A memory buffer is one contiguous allocation, so
// any in-bounds range can be answered without copying.
func (m MemoryBuffer) View(offset, length int) ([]byte, bool) {
	if offset < 0 || length < 0 || offset+length > len(m) {
		return nil, false
	}
	return m[offset : offset+length], true
}

type BufferType int

const (
	Original BufferType = iota
	Add
)

// Piece describes one text fragment.
type Piece struct {
	Buf    BufferType
	Start  int // Offset of the fragment start in the corresponding buffer
	Length int // Piece length
}

// PieceTable is a structure for efficient editing of large texts.
//
// The piece list is read from background goroutines — a search assembling the
// buffer it scans, the editor's line indexer walking the text — while the UI
// thread edits it. mu is what makes that safe. It guards the piece list, the
// add buffer's length and the size; the bytes those pieces point at never
// change once written, so a window handed out under the lock stays readable
// after it is released.
type PieceTable struct {
	mu     sync.RWMutex
	orig   Buffer  // Original (Read-only) buffer
	add    []byte  // Additive (Append-only) buffer
	pieces []Piece // Piece table
	size   int     // Current logical length of the entire text
}

// New creates a new piece table from original text.
func New(text []byte) *PieceTable {
	pt := &PieceTable{
		orig: MemoryBuffer(text),
		size: len(text),
	}
	if len(text) > 0 {
		pt.pieces = []Piece{{Buf: Original, Start: 0, Length: len(text)}}
	}
	return pt
}

// Size returns current logical length of the text.
func (pt *PieceTable) Size() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.size
}

// GetOriginalBuffer returns the underlying original buffer.
func (pt *PieceTable) GetOriginalBuffer() Buffer {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.orig
}

// offsetToPiece finds piece index and offset within it by global offset.
func (pt *PieceTable) offsetToPiece(offset int) (pieceIdx int, offsetInPiece int) {
	if offset == pt.size {
		return len(pt.pieces), 0
	}
	curr := 0
	for i, p := range pt.pieces {
		if offset < curr+p.Length {
			return i, offset - curr
		}
		curr += p.Length
	}
	return len(pt.pieces), 0
}

// Insert inserts data at the specified offset.
func (pt *PieceTable) Insert(offset int, data []byte) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if offset < 0 || offset > pt.size || len(data) == 0 {
		return
	}

	addStart := len(pt.add)
	pt.add = append(pt.add, data...)
	newPiece := Piece{Buf: Add, Start: addStart, Length: len(data)}

	// If the table is empty
	if pt.size == 0 {
		pt.pieces = []Piece{newPiece}
		pt.size += len(data)
		return
	}

	// Optimization: if inserting at the very end and previous piece is also Add — merge them
	if offset == pt.size && len(pt.pieces) > 0 {
		lastIdx := len(pt.pieces) - 1
		lastP := pt.pieces[lastIdx]
		if lastP.Buf == Add && lastP.Start+lastP.Length == addStart {
			pt.pieces[lastIdx].Length += len(data)
			pt.size += len(data)
			return
		}
		// Otherwise just append a new piece to the end
		pt.pieces = append(pt.pieces, newPiece)
		pt.size += len(data)
		return
	}

	// General case: insertion in the middle
	idx, off := pt.offsetToPiece(offset)
	p := pt.pieces[idx]

	var newPieces []Piece
	newPieces = append(newPieces, pt.pieces[:idx]...)

	if off == 0 {
		// Insertion exactly before the piece
		newPieces = append(newPieces, newPiece, p)
	} else {
		// Split the current piece into two
		left := Piece{Buf: p.Buf, Start: p.Start, Length: off}
		right := Piece{Buf: p.Buf, Start: p.Start + off, Length: p.Length - off}
		newPieces = append(newPieces, left, newPiece, right)
	}

	if idx+1 < len(pt.pieces) {
		newPieces = append(newPieces, pt.pieces[idx+1:]...)
	}

	pt.pieces = newPieces
	pt.size += len(data)
}

// Delete removes a text fragment of specified length starting from offset.
func (pt *PieceTable) Delete(offset, length int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if offset < 0 || length <= 0 || offset+length > pt.size {
		return
	}

	startIdx, startOff := pt.offsetToPiece(offset)
	endIdx, endOff := pt.offsetToPiece(offset + length)

	var newPieces []Piece
	newPieces = append(newPieces, pt.pieces[:startIdx]...)

	// Remainder of the left split piece
	if startOff > 0 {
		p := pt.pieces[startIdx]
		newPieces = append(newPieces, Piece{Buf: p.Buf, Start: p.Start, Length: startOff})
	}

	// Remainder of the right split piece
	if endIdx < len(pt.pieces) {
		p := pt.pieces[endIdx]
		if endOff < p.Length {
			newPieces = append(newPieces, Piece{Buf: p.Buf, Start: p.Start + endOff, Length: p.Length - endOff})
		}
	}

	// All pieces after endIdx
	if endIdx+1 < len(pt.pieces) {
		newPieces = append(newPieces, pt.pieces[endIdx+1:]...)
	}

	pt.pieces = newPieces
	pt.size -= length
}

// Bytes assembles and returns all current text.
// Note: for large file rendering in future we'll write ReadAt methods,
// so as not to unload entire buffer into memory.
func (pt *PieceTable) Bytes() ([]byte, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	res := make([]byte, 0, pt.size)
	for _, p := range pt.pieces {
		if p.Buf == Original {
			data, err := pt.orig.Read(p.Start, p.Length)
			if err != nil {
				return nil, err
			}
			res = append(res, data...)
		} else {
			res = append(res, pt.add[p.Start:p.Start+p.Length]...)
		}
	}
	return res, nil
}

// AppendRange appends the specified range to the dest slice without new allocations.
func (pt *PieceTable) AppendRange(dest []byte, offset, length int) ([]byte, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	if offset < 0 || length <= 0 {
		return dest, nil
	}
	if offset+length > pt.size {
		length = pt.size - offset
	}

	remaining := length
	startIdx, offInPiece := pt.offsetToPiece(offset)

	for i := startIdx; i < len(pt.pieces) && remaining > 0; i++ {
		p := pt.pieces[i]

		take := p.Length - offInPiece
		if take > remaining {
			take = remaining
		}

		if p.Buf == Original {
			data, err := pt.orig.Read(p.Start+offInPiece, take)
			if err != nil {
				return dest, err
			}
			dest = append(dest, data...)
		} else {
			dest = append(dest, pt.add[p.Start+offInPiece:p.Start+offInPiece+take]...)
		}

		remaining -= take
		offInPiece = 0
	}

	return dest, nil
}

// String returns current text as a string (convenient for tests).
// Ignore errors here to keep tests simple.
func (pt *PieceTable) String() string {
	b, _ := pt.Bytes()
	return string(b)
}

// ForEachRange sequentially calls a function for each data fragment.
// This allows processing text without allocating a single large slice.
//
// fn runs while the table is read-locked, so it must not edit the table it is
// walking; every caller here only reads what it is handed.
func (pt *PieceTable) ForEachRange(fn func(data []byte) error) error {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	for _, p := range pt.pieces {
		if p.Buf == Original {
			const chunkSize = 1024 * 1024
			for offset := 0; offset < p.Length; offset += chunkSize {
				take := chunkSize
				if offset+take > p.Length {
					take = p.Length - offset
				}
				data, err := pt.orig.Read(p.Start+offset, take)
				if err != nil {
					return err
				}
				if err := fn(data); err != nil {
					return err
				}
			}
		} else {
			if err := fn(pt.add[p.Start : p.Start+p.Length]); err != nil {
				return err
			}
		}
	}
	return nil
}

// TableState represents a snapshot of the PieceTable structure.
type TableState struct {
	Pieces []Piece
	Size   int
}

// GetState returns a snapshot of the current table structure.
func (pt *PieceTable) GetState() TableState {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	ps := make([]Piece, len(pt.pieces))
	copy(ps, pt.pieces)
	return TableState{Pieces: ps, Size: pt.size}
}

// LoadState restores the table structure from a snapshot.
func (pt *PieceTable) LoadState(s TableState) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.pieces = make([]Piece, len(s.Pieces))
	copy(pt.pieces, s.Pieces)
	pt.size = s.Size
}

// Equals compares two table states for structural identity.
func (s TableState) Equals(other TableState) bool {
	if s.Size != other.Size || len(s.Pieces) != len(other.Pieces) {
		return false
	}
	for i := range s.Pieces {
		if s.Pieces[i] != other.Pieces[i] {
			return false
		}
	}
	return true
}

// View returns a window into the text without copying it, and reports false
// when it cannot. It succeeds when the requested range lies inside a single
// piece whose backing buffer can provide a contiguous window: the add buffer
// always can, an Original buffer only if it implements Viewer.
//
// An unedited file is exactly one piece, so View(0, Size()) answers the whole
// buffer — which is what lets a search scan the file in place instead of
// assembling a copy of it. After edits the text is spread over several
// pieces and callers fall back to GetRange.
//
// The result aliases the buffer and must not be modified.
func (pt *PieceTable) View(offset, length int) ([]byte, bool) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	p, offInPiece, ok := pt.pieceHolding(offset, length)
	if !ok {
		return nil, false
	}
	if p.Buf == Add {
		start := p.Start + offInPiece
		return pt.add[start : start+length], true
	}

	v, ok := pt.orig.(Viewer)
	if !ok {
		return nil, false
	}
	return v.View(p.Start+offInPiece, length)
}

// pieceHolding returns the single piece that covers the whole of a range, and
// how far into it the range starts. It reports false for a range that is out
// of bounds or that crosses a piece boundary — which is what makes a window
// onto it possible at all. Callers hold the lock.
func (pt *PieceTable) pieceHolding(offset, length int) (Piece, int, bool) {
	if offset < 0 || length <= 0 || offset+length > pt.size {
		return Piece{}, 0, false
	}
	idx, offInPiece := pt.offsetToPiece(offset)
	if idx >= len(pt.pieces) {
		return Piece{}, 0, false
	}
	p := pt.pieces[idx]
	if offInPiece+length > p.Length {
		return Piece{}, 0, false
	}
	return p, offInPiece, true
}

// OriginalRange reports where a stretch of the text sits in the original
// buffer, and false when it does not sit there in one piece. It answers what
// View answers, minus the bytes.
//
// That is the whole point of it: a caller who has another way of reading the
// original — the descriptor of the file the buffer was made from — can read
// the range for itself instead of through the buffer. The answer is only yes
// for a range inside one untouched Original piece, so the bytes at that
// position are the bytes of that range no matter what has been typed elsewhere.
func (pt *PieceTable) OriginalRange(offset, length int) (int, bool) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	p, offInPiece, ok := pt.pieceHolding(offset, length)
	if !ok || p.Buf != Original {
		return 0, false
	}
	return p.Start + offInPiece, true
}

// GetRange returns a byte slice for the specified range.
func (pt *PieceTable) GetRange(offset, length int) ([]byte, error) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	if offset < 0 || length <= 0 || offset+length > pt.size {
		return nil, nil
	}

	res := make([]byte, 0, length)
	remaining := length

	startIdx, offInPiece := pt.offsetToPiece(offset)

	for i := startIdx; i < len(pt.pieces) && remaining > 0; i++ {
		p := pt.pieces[i]

		// Determine how much data we take from this piece
		take := p.Length - offInPiece
		if take > remaining {
			take = remaining
		}

		var buf []byte
		if p.Buf == Original {
			var err error
			buf, err = pt.orig.Read(p.Start+offInPiece, take)
			if err != nil {
				return nil, err
			}
		} else {
			buf = pt.add[p.Start+offInPiece : p.Start+offInPiece+take]
		}

		res = append(res, buf...)

		remaining -= take
		offInPiece = 0 // For subsequent pieces, read from start
	}

	return res, nil
}

// UpdateOriginalBuffer safely replaces the original underlying buffer
// without losing the current logical state and additions.
// Used primarily for state recovery after a failed I/O operation.
func (pt *PieceTable) UpdateOriginalBuffer(buf Buffer) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.orig = buf
}
func NewWithBuffer(buf Buffer) *PieceTable {
	pt := &PieceTable{
		orig: buf,
		size: buf.Size(),
	}
	if buf.Size() > 0 {
		pt.pieces = []Piece{{Buf: Original, Start: 0, Length: buf.Size()}}
	}
	return pt
}
