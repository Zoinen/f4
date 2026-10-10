package far2ldnd

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// Terminator ends an APC. far2l sends BEL in interactions; both parsers
// accept ST as well (§ 2.1).
type Terminator int

const (
	BEL Terminator = iota // 0x07
	ST                    // ESC \
)

func (t Terminator) String() string {
	if t == ST {
		return "\x1b\\"
	}
	return "\x07"
}

// FrameKind tells the three stack-carrying APCs apart.
type FrameKind int

const (
	FrameRequest FrameKind = iota + 1 // ESC _ far2l: B64 — application to terminal
	FrameReply                        // ESC _ far2l B64 — terminal to application
	FrameEvent                        // ESC _ f2l B64 — terminal to application
)

func (k FrameKind) prefix() string {
	switch k {
	case FrameRequest:
		return "\x1b_far2l:"
	case FrameReply:
		return replyIntro
	case FrameEvent:
		return "\x1b_f2l"
	}
	panic(fmt.Sprintf("far2ldnd: frame kind %d", int(k)))
}

// Frame wraps a stack into a complete APC. The stack is base64-encoded once;
// file bytes inside it are plain bytes, not a second base64.
func Frame(k FrameKind, stack []byte, t Terminator) string {
	return k.prefix() + base64.StdEncoding.EncodeToString(stack) + t.String()
}

// FrameLen is the length of Frame(k, stack, t) for a stack of n bytes: the
// quantity max_frame limits (introducer, base64 and terminator included).
func FrameLen(k FrameKind, n int, t Terminator) int {
	return len(k.prefix()) + base64.StdEncoding.EncodedLen(n) + len(t.String())
}

// replyIntro opens a reply APC.
const replyIntro = "\x1b_far2l"

// readReplyOverhead is RID, status, observed_size, read_flags and length.
const readReplyOverhead = 1 + 1 + 8 + 1 + 4

// MaxReadData is the largest READ payload whose complete reply still fits in
// maxFrame when sent with terminator t (§ 7: the chunk shrinks so that the
// reply is guaranteed to fit). Zero means not even an empty reply fits.
func MaxReadData(maxFrame uint32, t Terminator) uint32 {
	fixed := uint32(len(replyIntro)) + 1
	if t == ST {
		fixed++
	}
	if maxFrame <= fixed {
		return 0
	}
	stack := (maxFrame - fixed) / 4 * 3
	if stack <= readReplyOverhead {
		return 0
	}
	return stack - readReplyOverhead
}

// errorReplyOverhead is RID, status and the message's own length prefix.
const errorReplyOverhead = 1 + 1 + 4

// MaxErrorText is the largest message whose EncodeError(rid, status, ·)
// reply still fits in maxFrame when sent with terminator t -- the same
// shrink MaxReadData works out for a READ reply, for an error reply's
// layout instead (RID, status, str message). BindFrameLimit before BIND
// caps it well under EncodeError's own flat MaxMessageLen ceiling (owner's
// answer 3): 369 bytes at ST, 372 at BEL.
func MaxErrorText(maxFrame uint32, t Terminator) int {
	fixed := uint32(len(replyIntro)) + uint32(len(t.String())) //nolint:gosec // replyIntro and t.String() are short fixed constants, never near MaxUint32
	if maxFrame <= fixed {
		return 0
	}
	budget := maxFrame - fixed
	stack := budget / 4 * 3
	if stack <= errorReplyOverhead {
		return 0
	}
	return int(stack - errorReplyOverhead) //nolint:gosec // bounded by maxFrame, far below MaxInt
}

// ParseFrame takes a complete APC carrying a stack apart. Control APCs such
// as far2l1, far2lok and far2l0 carry no stack and are refused, as is
// anything that is not base64 after the prefix. Missing base64 padding is
// tolerated, as the rest of f4 does.
func ParseFrame(apc string) (FrameKind, []byte, Terminator, error) {
	var t Terminator
	switch {
	case strings.HasSuffix(apc, ST.String()):
		t = ST
	case strings.HasSuffix(apc, BEL.String()):
		t = BEL
	default:
		return 0, nil, 0, fmt.Errorf("%w: APC without BEL or ST", ErrInvalid)
	}
	body := strings.TrimSuffix(apc, t.String())
	switch body {
	case "\x1b_far2l1", "\x1b_far2l0", "\x1b_far2lok":
		// "ok" would even pass for base64.
		return 0, nil, t, fmt.Errorf("%w: far2l control APC carries no stack", ErrInvalid)
	}
	var k FrameKind
	// far2l: is tested before far2l, which is its prefix.
	for _, c := range []FrameKind{FrameRequest, FrameReply, FrameEvent} {
		if strings.HasPrefix(body, c.prefix()) {
			k = c
			break
		}
	}
	if k == 0 {
		return 0, nil, t, fmt.Errorf("%w: not a far2l stack APC", ErrInvalid)
	}
	b64 := strings.TrimPrefix(body, k.prefix())
	if m := len(b64) % 4; m != 0 {
		b64 += strings.Repeat("=", 4-m)
	}
	stack, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(stack) == 0 {
		return 0, nil, t, fmt.Errorf("%w: APC payload is not a base64 stack", ErrInvalid)
	}
	return k, stack, t, nil
}
