// Package far2ldnd is the wire codec of the terminal drag-and-drop protocol
// that rides on far2l extensions (unxed/f4#1628, specification v0.2).
//
// A drop reaches the application as a short INPUT_DND event carrying an
// opaque offer; the application then pulls the list of dropped objects and
// the byte ranges it wants through ordinary far2l interaction requests of one
// new family: DND/BIND, DND/LIST, DND/READ and DND/CLOSE.
//
// The package only turns those messages into far2l stacks and back, strictly
// by the byte layout of the specification, and checks every rule that can be
// judged from a single message. It keeps no state: RID bookkeeping, the
// dispatcher, lease timers, frame accumulation and the VFS adapter are the
// next steps of the plan and live elsewhere. It is placed next to
// internal/terminal, which serves far2l extensions to the programs running
// inside f4, but it imports nothing from it, so the client side (f4 running
// inside a terminal) can use it as well.
//
// # The far2l stack
//
// far2l's StackSerializer is a LIFO buffer: integers are little-endian, a
// reader pops fields from the END of the buffer, and a string is its bytes
// followed by a uint32 length, so the length is popped first. Every field
// list in the specification, and in this package, is in pop order. The
// writer below takes the fields in that same order and lays them out
// reversed, so each encoder reads like the table it implements.
package far2ldnd

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

// Errors of the codec. A decoding error of a request maps to a reply status
// through StatusFor.
var (
	// ErrTruncated: the stack ends before a field does.
	ErrTruncated = errors.New("far2ldnd: stack truncated")
	// ErrTrailing: bytes are left in the stack after the last field.
	ErrTrailing = errors.New("far2ldnd: unexpected trailing bytes")
	// ErrTooLong: a length prefix exceeds the limit of its field.
	ErrTooLong = errors.New("far2ldnd: field longer than allowed")
	// ErrInvalidUTF8: a str field is not UTF-8.
	ErrInvalidUTF8 = errors.New("far2ldnd: str field is not valid UTF-8")
	// ErrInvalid: a field breaks a rule of the specification.
	ErrInvalid = errors.New("far2ldnd: invalid message")
	// ErrNotDND: the interaction is not of the DND family.
	ErrNotDND = errors.New("far2ldnd: not a DND interaction")
	// ErrUnknownSubcommand: a DND subcommand this version does not know.
	ErrUnknownSubcommand = errors.New("far2ldnd: unknown DND subcommand")
	// ErrUnsupportedVersion: BIND asked for a protocol version other than 1.
	ErrUnsupportedVersion = errors.New("far2ldnd: unsupported protocol version")
	// ErrNoDND: the reply is empty after its RID -- the answer of a far2l
	// server that does not know the command, i.e. no DND on the other side.
	ErrNoDND = errors.New("far2ldnd: the terminal does not support DND")
)

// ID is a 16-byte binding or offer identifier. On the stack it has no length
// prefix; its bytes keep their order.
type ID [16]byte

// stackWriter collects fields in pop order and lays them out reversed.
type stackWriter struct {
	parts [][]byte
	err   error
}

func (w *stackWriter) raw(b []byte) { w.parts = append(w.parts, b) }

func (w *stackWriter) u8(v uint8) { w.raw([]byte{v}) }

func (w *stackWriter) i8(v int8) { w.u8(uint8(v)) } //nolint:gosec // two's complement reinterpretation is the wire format

func (w *stackWriter) u16(v uint16) { w.raw(binary.LittleEndian.AppendUint16(nil, v)) }

func (w *stackWriter) i16(v int16) { w.u16(uint16(v)) } //nolint:gosec // two's complement reinterpretation is the wire format

func (w *stackWriter) u32(v uint32) { w.raw(binary.LittleEndian.AppendUint32(nil, v)) }

func (w *stackWriter) u64(v uint64) { w.raw(binary.LittleEndian.AppendUint64(nil, v)) }

func (w *stackWriter) id(v ID) { w.raw(append([]byte(nil), v[:]...)) }

// blob writes a length-prefixed byte string: popped as the uint32 length,
// then the bytes, so physically the bytes come first.
func (w *stackWriter) blob(b []byte) {
	n := uint64(len(b))
	if n > math.MaxUint32 {
		w.fail(fmt.Errorf("%w: %d bytes", ErrTooLong, n))
		return
	}
	w.u32(uint32(n)) //nolint:gosec // bounded by the check above
	w.raw(append([]byte(nil), b...))
}

func (w *stackWriter) str(s string) {
	if !utf8.ValidString(s) {
		w.fail(ErrInvalidUTF8)
		return
	}
	w.blob([]byte(s))
}

func (w *stackWriter) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

func (w *stackWriter) bytes() ([]byte, error) {
	if w.err != nil {
		return nil, w.err
	}
	n := 0
	for _, p := range w.parts {
		n += len(p)
	}
	out := make([]byte, 0, n)
	for i := len(w.parts) - 1; i >= 0; i-- {
		out = append(out, w.parts[i]...)
	}
	return out, nil
}

// stackReader pops fields from the end of a stack. The first error sticks;
// every later pop returns zero values.
type stackReader struct {
	b   []byte
	err error
}

func (r *stackReader) take(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n > len(r.b) {
		r.err = ErrTruncated
		return nil
	}
	v := r.b[len(r.b)-n:]
	r.b = r.b[:len(r.b)-n]
	return v
}

func (r *stackReader) u8() uint8 {
	if v := r.take(1); v != nil {
		return v[0]
	}
	return 0
}

func (r *stackReader) i8() int8 { return int8(r.u8()) } //nolint:gosec // two's complement reinterpretation is the wire format

func (r *stackReader) u16() uint16 {
	if v := r.take(2); v != nil {
		return binary.LittleEndian.Uint16(v)
	}
	return 0
}

func (r *stackReader) i16() int16 { return int16(r.u16()) } //nolint:gosec // two's complement reinterpretation is the wire format

func (r *stackReader) u32() uint32 {
	if v := r.take(4); v != nil {
		return binary.LittleEndian.Uint32(v)
	}
	return 0
}

func (r *stackReader) u64() uint64 {
	if v := r.take(8); v != nil {
		return binary.LittleEndian.Uint64(v)
	}
	return 0
}

func (r *stackReader) id() ID {
	var v ID
	if b := r.take(len(v)); b != nil {
		copy(v[:], b)
	}
	return v
}

// blob pops a length-prefixed byte string. The length is checked against the
// field limit and against what is left of the stack before anything is
// allocated.
func (r *stackReader) blob(limit uint64) []byte {
	n := r.u32()
	if r.err != nil {
		return nil
	}
	if uint64(n) > limit {
		r.err = fmt.Errorf("%w: %d bytes, limit %d", ErrTooLong, n, limit)
		return nil
	}
	if uint64(n) > uint64(len(r.b)) {
		r.err = ErrTruncated
		return nil
	}
	v := r.take(int(n)) //nolint:gosec // n <= len(r.b), checked above
	return append(make([]byte, 0, len(v)), v...)
}

func (r *stackReader) str(limit uint64) string {
	b := r.blob(limit)
	if r.err != nil {
		return ""
	}
	if !utf8.Valid(b) {
		r.err = ErrInvalidUTF8
		return ""
	}
	return string(b)
}

func (r *stackReader) invalid(format string, args ...any) {
	if r.err == nil {
		r.err = fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)
	}
}

// end requires the stack to be fully consumed.
func (r *stackReader) end() error {
	if r.err == nil && len(r.b) != 0 {
		r.err = fmt.Errorf("%w: %d bytes", ErrTrailing, len(r.b))
	}
	return r.err
}
