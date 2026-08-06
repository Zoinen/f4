package fishplus

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
)

// MaxPatchSegments caps one patch request. An editor that produced more
// pieces than this has effectively rewritten the file, and writing it out
// whole is then both simpler and faster.
const MaxPatchSegments = 100000

// PatchSegment is one piece of the file being built. Data set means literal
// new bytes; Data nil means Length bytes taken from the original file at
// Offset, which never leave the remote host.
type PatchSegment struct {
	Offset int64
	Length int64
	Data   []byte
}

// Copy describes a range of the original file.
func Copy(off, length int64) PatchSegment { return PatchSegment{Offset: off, Length: length} }

// Literal describes bytes that have to be sent.
func Literal(data []byte) PatchSegment {
	return PatchSegment{Length: int64(len(data)), Data: data}
}

// CanPatch reports whether the remote host can be asked to assemble a file.
// It needs a write backend for the literals and dd for the copying.
func (c *Client) CanPatch() bool {
	return c.WriteMode() != "" && c.sess.Features().Has("dd")
}

// Patch builds dst out of the segments, reading the copied ranges from src
// on the remote host. src and dst may not be the same file: the result is
// written forward from nothing, so a segment could otherwise read a range
// that has already been overwritten.
//
// This is what makes an edit cheap. A one byte change in a hundred megabyte
// file is two copies and one literal, so one byte crosses the network while
// the remote host does the copying at local disk speed.
func (c *Client) Patch(ctx context.Context, src, dst string, segs []PatchSegment) error {
	if src == dst {
		return fmt.Errorf("fishplus: patch %q: source and destination must differ", dst)
	}
	mode := c.WriteMode()
	if mode == "" {
		return ErrNoWrite
	}
	encoded := mode == "b64"
	enc := "raw"
	if encoded {
		enc = "b64"
	}

	// A literal larger than one chunk is split, so that the b64 backend
	// never has to read a line of unbounded length and the raw one never
	// holds a whole file in a single request.
	limit := int64(c.WriteChunk())
	planned := make([]PatchSegment, 0, len(segs))
	for _, seg := range segs {
		if seg.Data == nil {
			if seg.Offset < 0 || seg.Length < 0 {
				return fmt.Errorf("fishplus: patch %q: bad range %d+%d", dst, seg.Offset, seg.Length)
			}
			if seg.Length == 0 {
				continue
			}
			planned = append(planned, seg)
			continue
		}
		for off := int64(0); off < int64(len(seg.Data)); off += limit {
			end := off + limit
			if end > int64(len(seg.Data)) {
				end = int64(len(seg.Data))
			}
			planned = append(planned, Literal(seg.Data[off:end]))
		}
	}
	if len(planned) > MaxPatchSegments {
		return fmt.Errorf("fishplus: patch %q: %d segments exceed %d", dst, len(planned), MaxPatchSegments)
	}

	resp, err := c.sess.ExecStream(ctx, "patch", []string{src, dst},
		[]string{strconv.Itoa(len(planned)), enc}, func(w io.Writer) error {
			for _, seg := range planned {
				if seg.Data == nil {
					if _, err := fmt.Fprintf(w, "S %d %d\n", seg.Offset, seg.Length); err != nil {
						return err
					}
					continue
				}
				if _, err := fmt.Fprintf(w, "D %d\n", len(seg.Data)); err != nil {
					return err
				}
				if encoded {
					line := base64.StdEncoding.EncodeToString(seg.Data) + "\n"
					if _, err := io.WriteString(w, line); err != nil {
						return err
					}
					continue
				}
				if _, err := w.Write(seg.Data); err != nil {
					return err
				}
			}
			return nil
		})
	if err != nil {
		return err
	}
	if resp.OK() {
		return nil
	}
	// As with a write: without the drained marker an unknown number of
	// payload bytes are still on the wire, and the next request would be
	// parsed out of the middle of a file.
	if !hasLine(resp.Lines, drainedLine) {
		c.sess.MarkBroken()
	}
	return resp.Err("patch " + dst)
}
