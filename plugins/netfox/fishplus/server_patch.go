package fishplus

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// patchSegment is one piece of the file being built: a range of the source
// (data nil) or literal bytes that came with the request.
type patchSegment struct {
	off, length int64
	data        []byte
}

// servePatch answers patch: <nsegs> raw|b64, two path lines (source and
// destination), then one descriptor per segment -- "S <offset> <length>" for a
// range of the source, "D <length>" followed by the literal bytes (raw, or one
// base64 line). Everything is read before anything is done, so a refusal still
// leaves the stream at a request boundary and carries the "D" line that tells
// the client so; the destination is written forward from nothing, which is why
// it may not be the source.
func (srv *Server) servePatch(in *bufio.Reader, w io.Writer, token, id string, args []string) error {
	n, okN := atoiArg(args, 0)
	enc := ""
	if len(args) > 1 {
		enc = args[1]
	}
	if !okN || n > MaxPatchSegments || (enc != "raw" && enc != "b64") {
		_ = end0(w, token, id, "err", "bad patch request")
		return errUnrecoverable
	}
	src, err := readServerPath(in)
	if err != nil {
		return err
	}
	dst, err := readServerPath(in)
	if err != nil {
		return err
	}
	segs := make([]patchSegment, 0, n)
	for i := 0; i < n; i++ {
		line, err := readServerLine(in)
		if err != nil {
			return err
		}
		f := strings.Fields(line)
		switch {
		case len(f) == 3 && f[0] == "S":
			off, err1 := strconv.ParseInt(f[1], 10, 64)
			length, err2 := strconv.ParseInt(f[2], 10, 64)
			if err1 != nil || err2 != nil || off < 0 || length < 0 {
				_ = end0(w, token, id, "err", "bad patch segment")
				return errUnrecoverable
			}
			segs = append(segs, patchSegment{off: off, length: length})
		case len(f) == 2 && f[0] == "D":
			length, err := strconv.Atoi(f[1])
			if err != nil || length < 0 || length > MaxWriteLen {
				_ = end0(w, token, id, "err", "bad patch segment")
				return errUnrecoverable
			}
			var data []byte
			if enc == "raw" {
				data = make([]byte, length)
				if _, err := io.ReadFull(in, data); err != nil {
					return err
				}
			} else {
				b64, err := readServerLine(in)
				if err != nil {
					return err
				}
				if data, err = base64.StdEncoding.DecodeString(b64); err != nil || len(data) != length {
					data = nil
					segs = append(segs, patchSegment{off: -1}) // marks a bad payload, reported below
					continue
				}
			}
			segs = append(segs, patchSegment{length: int64(length), data: data})
		default:
			_ = end0(w, token, id, "err", "bad patch segment")
			return errUnrecoverable
		}
	}
	if opErr := buildPatch(src, dst, segs); opErr != nil {
		_, _ = fmt.Fprintf(w, "D\n")
		return end0(w, token, id, "err", errText(opErr))
	}
	return end0(w, token, id, "ok", "")
}

func buildPatch(srcPath, dstPath string, segs []patchSegment) error {
	srcPath, err := guardPath(srcPath)
	if err != nil {
		return err
	}
	dstPath, err = guardPath(dstPath)
	if err != nil {
		return err
	}
	if srcPath == dstPath {
		return fmt.Errorf("patch %s: source and destination must differ", dstPath)
	}
	for _, s := range segs {
		if s.off == -1 && s.data == nil && s.length == 0 {
			return fmt.Errorf("patch %s: bad payload", dstPath)
		}
	}
	var src *os.File
	needSrc := false
	for _, s := range segs {
		if s.data == nil && s.length > 0 {
			needSrc = true
		}
	}
	if needSrc {
		if src, err = os.Open(srcPath); err != nil { //nolint:gosec // the client's path, guarded above
			return err
		}
		defer func() { _ = src.Close() }()
	}
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // the client's path, guarded above
	if err != nil {
		return err
	}
	for _, s := range segs {
		if s.data != nil {
			if _, err := dst.Write(s.data); err != nil {
				_ = dst.Close()
				return err
			}
			continue
		}
		if s.length == 0 {
			continue
		}
		if _, err := io.Copy(dst, io.NewSectionReader(src, s.off, s.length)); err != nil {
			_ = dst.Close()
			return err
		}
	}
	return dst.Close()
}
