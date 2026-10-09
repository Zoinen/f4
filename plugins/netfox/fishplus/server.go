package fishplus

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Server speaks the FISH+ wire protocol (docs/FISH+.md) from Go, so that a
// machine with f4 installed needs no shell helper: the client starts f4 as the
// remote command, says hello with BootstrapNative and from then on talks to it
// exactly as it talks to helper.sh. It is the first step of using f4 as the
// remote server (unxed/f4#1680); only the session commands are served so far --
// noop, pwd, ping, feats and exit, plus info, linfo, enum, rdlink, isdirs, read the mutations and writes (write, mkdir, rm, rmdir, rmtree, mv, cp, mklink, chmod, chown, utime, trunc),
// which answer in the "find" listing format -- and every other command of the protocol
// answers "unknown command" after its path lines have been read, so a client
// that has probed the banner's features never desynchronises the stream.
type Server struct {
	// Dir is what pwd reports; empty means the process' working directory.
	Dir string
	// Features are the words after the protocol version in the banner.
	Features []string
}

// serverFeatures is what a Server announces when none is set: the native
// marker, which tells a client that no shell tool is behind the answers.
var serverFeatures = []string{"native", "mode:find", "read:ddbytes", "write:ddbytes", "ln", "truncate", "dd"}

// pathLines is how many path lines follow the request line of each command of
// the protocol, which is what a server has to consume to stay in step with a
// command it does not implement. Commands with a payload after the paths
// (write, patch) are absent: their length cannot be skipped blindly.
var pathLines = map[string]int{
	"noop": 0, "pwd": 0, "ping": 1, "feats": 0, "exit": 0, "enum": 1, "info": 1, "linfo": 1,
	"rdlink": 1, "read": 1, "trunc": 1, "mkdir": 1, "rm": 1, "rmdir": 1, "rmtree": 1,
	"mv": 2, "cp": 2, "mklink": 2, "chmod": 1, "chown": 1, "utime": 1, "grep": 2, "lidx": 1,
	"jpoll": 0, "jkill": 0, "jdrop": 0, "jlist": 0, "isdirs": 0, "mode": 0, "rmode": 0, "wmode": 0,
}

// errUnrecoverable ends a session whose stream a server can no longer follow.
var errUnrecoverable = errors.New("fishplus: request cannot be skipped")

// Serve runs one session over the duplex stream until the client says exit or
// closes it. The first line must be the client's hello (NativeHelloLine).
func (srv *Server) Serve(r io.Reader, w io.Writer) error {
	in := bufio.NewReader(r)
	hello, err := readServerLine(in)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(hello, NativeHelloPrefix) || len(hello) == len(NativeHelloPrefix) {
		return fmt.Errorf("fishplus: bad hello %q", hello)
	}
	token := strings.TrimSpace(hello[len(NativeHelloPrefix):])
	feats := srv.Features
	if len(feats) == 0 {
		feats = serverFeatures
	}
	banner := fmt.Sprintf("FISHPLUS %d %s", ProtocolVersion, strings.Join(feats, " "))
	// The newline in front is the same courtesy helper.sh pays, so the client's
	// lenient handshake matching sees the terminator at the start of a line.
	if _, err := fmt.Fprintf(w, "\n.%s 0 ok %s\n", token, banner); err != nil {
		return err
	}

	for {
		line, err := readServerLine(in)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return fmt.Errorf("fishplus: bad request %q", line)
		}
		id, cmd := fields[0], fields[1]
		if _, err := strconv.ParseUint(id, 10, 64); err != nil {
			return fmt.Errorf("fishplus: bad request id %q", id)
		}
		if cmd == "write" {
			if err := srv.serveWrite(in, w, token, id, fields[2:]); err != nil {
				return err
			}
			continue
		}
		if cmd == "patch" {
			if err := srv.servePatch(in, w, token, id, fields[2:]); err != nil {
				return err
			}
			continue
		}
		count, known := pathLines[cmd]
		if cmd == "isdirs" {
			n, ok := atoiArg(fields[2:], 0)
			if !ok {
				_ = end0(w, token, id, "err", "bad path count")
				return errUnrecoverable
			}
			count, known = n, true
		}
		paths := make([]string, 0, count)
		for i := 0; i < count; i++ {
			p, err := readServerPath(in)
			if err != nil {
				return err
			}
			paths = append(paths, p)
		}
		end := func(status, msg string) error { return end0(w, token, id, status, msg) }
		// reply sends payload lines and then the terminator: ok, or err with
		// the reason when the operation failed.
		reply := func(lines []string, opErr error) error {
			if opErr != nil {
				return end("err", errText(opErr))
			}
			for _, l := range lines {
				if _, err := fmt.Fprintf(w, "%s\n", l); err != nil {
					return err
				}
			}
			return end("ok", "")
		}
		switch cmd {
		case "noop":
			err = end("ok", "")
		case "pwd":
			dir := srv.Dir
			if dir == "" {
				if dir, err = os.Getwd(); err != nil {
					err = end("err", err.Error())
					break
				}
			}
			if _, err = fmt.Fprintf(w, "%s\n", dir); err == nil {
				err = end("ok", "")
			}
		case "ping":
			if _, err = fmt.Fprintf(w, "%s\n", paths[0]); err == nil {
				err = end("ok", "")
			}
		case "feats":
			if _, err = fmt.Fprintf(w, "%d %s\n", ProtocolVersion, strings.Join(feats, " ")); err == nil {
				err = end("ok", "")
			}
		case "info", "linfo":
			lines, opErr := infoLines(paths[0], cmd == "info")
			err = reply(lines, opErr)
		case "enum":
			lines, opErr := enumLines(paths[0])
			err = reply(lines, opErr)
		case "rdlink":
			target, opErr := os.Readlink(paths[0])
			err = reply([]string{target}, opErr)
		case "isdirs":
			err = reply(isdirsLines(paths), nil)
		case "read":
			off, okOff := atoiArg(fields[2:], 0)
			length, okLen := atoiArg(fields[2:], 1)
			if !okOff || !okLen {
				err = end("err", "bad range")
				break
			}
			size, data, opErr := readRange(paths[0], int64(off), int64(length))
			if opErr != nil {
				err = end("err", errText(opErr))
				break
			}
			if _, err = fmt.Fprintf(w, "S %d\n", size); err != nil {
				break
			}
			if len(data) > 0 {
				if _, err = fmt.Fprintf(w, "#%d\n", len(data)); err != nil {
					break
				}
				if _, err = w.Write(data); err != nil {
					break
				}
			}
			err = end("ok", "")
		case "mkdir", "rm", "rmdir", "rmtree", "mv", "cp", "mklink", "chmod", "chown", "utime", "trunc":
			err = reply(nil, mutate(cmd, fields[2:], paths))
		case "exit":
			return end("ok", "")
		default:
			if !known {
				// The stream cannot be followed past a command whose payload
				// or path count is not known.
				_ = end("err", "unknown command")
				return errUnrecoverable
			}
			err = end("err", "unknown command")
		}
		if err != nil {
			return err
		}
	}
}

func end0(w io.Writer, token, id, status, msg string) error {
	if msg != "" {
		msg = " " + msg
	}
	_, err := fmt.Fprintf(w, ".%s %s %s%s\n", token, id, status, msg)
	return err
}

func readServerLine(in *bufio.Reader) (string, error) {
	line, err := in.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readServerPath reads one path line and undoes the "~" base64 escape a client
// applies to a path a line cannot carry raw (EncodePathLine).
func readServerPath(in *bufio.Reader) (string, error) {
	line, err := readServerLine(in)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(line, "~") {
		raw, err := base64.StdEncoding.DecodeString(line[1:])
		if err != nil {
			return "", fmt.Errorf("fishplus: bad escaped path: %w", err)
		}
		return string(raw), nil
	}
	return line, nil
}

// mutate runs one of the mutating commands.
func mutate(cmd string, args, paths []string) error {
	arg := func(i int) string {
		if i < len(args) {
			return args[i]
		}
		return ""
	}
	switch cmd {
	case "mkdir":
		return mutMkdir(paths[0])
	case "rm":
		return mutRemove(paths[0], false)
	case "rmdir":
		return mutRemove(paths[0], true)
	case "rmtree":
		return mutRemoveAll(paths[0])
	case "mv":
		return mutRename(paths[0], paths[1])
	case "cp":
		return mutCopy(paths[0], paths[1])
	case "mklink":
		return mutSymlink(paths[0], paths[1])
	case "chmod":
		return mutChmod(paths[0], arg(0))
	case "chown":
		return mutChown(paths[0], arg(0), arg(1))
	case "utime":
		return mutUtime(paths[0], arg(0), arg(1))
	case "trunc":
		return mutTruncate(paths[0], arg(0))
	}
	return fmt.Errorf("unknown command")
}

// serveWrite answers write: <offset> <length> raw|b64, one path line, then the
// payload -- exactly length raw bytes, or one base64 line. The payload is
// always consumed, even when the write is refused, so that the stream stays at
// a request boundary; a refusal after that carries the "D" line that tells the
// client the stream is intact.
func (srv *Server) serveWrite(in *bufio.Reader, w io.Writer, token, id string, args []string) error {
	off, okOff := atoiArg(args, 0)
	length, okLen := atoiArg(args, 1)
	enc := ""
	if len(args) > 2 {
		enc = args[2]
	}
	if !okOff || !okLen || (enc != "raw" && enc != "b64") || length > MaxWriteLen {
		_ = end0(w, token, id, "err", "bad write request")
		return errUnrecoverable
	}
	path, err := readServerPath(in)
	if err != nil {
		return err
	}
	var data []byte
	if enc == "raw" {
		data = make([]byte, length)
		if _, err := io.ReadFull(in, data); err != nil {
			return err
		}
	} else {
		line, err := readServerLine(in)
		if err != nil {
			return err
		}
		data, err = base64.StdEncoding.DecodeString(line)
		if err != nil || len(data) != length {
			_, _ = fmt.Fprintf(w, "D\n")
			return end0(w, token, id, "err", "bad payload")
		}
	}
	if opErr := writeAt(path, int64(off), data); opErr != nil {
		_, _ = fmt.Fprintf(w, "D\n")
		return end0(w, token, id, "err", errText(opErr))
	}
	return end0(w, token, id, "ok", "")
}
