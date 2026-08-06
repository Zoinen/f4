package fishplus

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Raw st_mode bits, spelled out here instead of taken from syscall so the
// package keeps building on every platform f4 targets.
const (
	modeFmt  = 0170000
	modeFifo = 0010000
	modeChar = 0020000
	modeDir  = 0040000
	modeBlk  = 0060000
	modeReg  = 0100000
	modeLink = 0120000
	modeSock = 0140000
)

// Entry is one directory entry as reported by the remote host.
type Entry struct {
	Name  string
	Size  int64
	Mode  uint32 // raw st_mode, file type bits included
	MTime time.Time
	ATime time.Time
	CTime time.Time
	Uid   int
	Gid   int
	// TargetIsDir tells whether a symlink points at a directory. Only the
	// "find" listing mode reports it for free; in the other modes it stays
	// false and the caller has to resolve the link with Stat if it cares.
	TargetIsDir bool
}

func (e Entry) IsDir() bool     { return e.Mode&modeFmt == modeDir }
func (e Entry) IsSymlink() bool { return e.Mode&modeFmt == modeLink }
func (e Entry) IsRegular() bool { return e.Mode&modeFmt == modeReg }

// Perm returns the permission and setuid/setgid/sticky bits.
func (e Entry) Perm() uint32 { return e.Mode & 07777 }

// IsExecutable reports whether any execute bit is set.
func (e Entry) IsExecutable() bool { return e.Mode&0111 != 0 }

// ListingModes are the metadata backends the helper can drive, ordered by
// preference: GNU find carries everything in a single call and even tells
// where a symlink points, while the stat flavours need a shell glob and
// report the link itself only.
var ListingModes = []string{"find", "stat", "statbsd", "ls"}

func typeCharMode(c byte) uint32 {
	switch c {
	case 'f':
		return modeReg
	case 'd':
		return modeDir
	case 'l':
		return modeLink
	case 'b':
		return modeBlk
	case 'c':
		return modeChar
	case 'p':
		return modeFifo
	case 's':
		return modeSock
	}
	return 0
}

// parseEpoch understands both plain seconds ("1785869231") and the
// fractional form GNU find prints ("1785869231.1084370490").
func parseEpoch(s string) (time.Time, error) {
	secText, fracText, _ := strings.Cut(s, ".")
	sec, err := strconv.ParseInt(secText, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad timestamp %q", s)
	}
	var nsec int64
	if fracText != "" {
		if len(fracText) > 9 {
			fracText = fracText[:9]
		}
		for len(fracText) < 9 {
			fracText += "0"
		}
		nsec, err = strconv.ParseInt(fracText, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad timestamp %q", s)
		}
	}
	return time.Unix(sec, nsec), nil
}

func parseInt(field, s string) (int64, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad %s %q", field, s)
	}
	return v, nil
}

// parseFindEntry reads a line produced with
// -printf '%y %Y %s %T@ %A@ %C@ %m %U %G %f\n'
func parseFindEntry(line string) (Entry, error) {
	f := strings.SplitN(line, " ", 10)
	if len(f) < 10 || f[0] == "" {
		return Entry{}, fmt.Errorf("find listing: expected 10 fields in %q", line)
	}
	var e Entry
	var err error
	size, err := parseInt("size", f[2])
	if err != nil {
		return Entry{}, err
	}
	perm, err := strconv.ParseUint(f[6], 8, 32)
	if err != nil {
		return Entry{}, fmt.Errorf("find listing: bad mode %q", f[6])
	}
	uid, err := parseInt("uid", f[7])
	if err != nil {
		return Entry{}, err
	}
	gid, err := parseInt("gid", f[8])
	if err != nil {
		return Entry{}, err
	}
	if e.MTime, err = parseEpoch(f[3]); err != nil {
		return Entry{}, err
	}
	if e.ATime, err = parseEpoch(f[4]); err != nil {
		return Entry{}, err
	}
	if e.CTime, err = parseEpoch(f[5]); err != nil {
		return Entry{}, err
	}
	e.Size = size
	e.Mode = uint32(perm) | typeCharMode(f[0][0])
	e.Uid = int(uid)
	e.Gid = int(gid)
	e.TargetIsDir = f[1] == "d"
	e.Name = f[9]
	return e, nil
}

// parseStatEntry reads a line produced with GNU
// stat -c '%f %s %Y %X %Z %u %g %n', where the mode comes in hex.
func parseStatEntry(line string) (Entry, error) {
	return parseStatLike(line, 16, "stat listing", false)
}

// parseBSDStatEntry reads a line produced with BSD
// stat -f '%p %z %m %a %c %u %g %N', where the mode comes in octal.
func parseBSDStatEntry(line string) (Entry, error) {
	return parseStatLike(line, 8, "bsd stat listing", false)
}

// keepPath decides what the last field means. A directory listing wants the
// bare name, because that is what a panel shows; a tree search answers with
// paths, and reducing one to its base name loses the only thing that says
// where the hit is.
func parseStatLike(line string, modeBase int, what string, keepPath bool) (Entry, error) {
	f := strings.SplitN(line, " ", 8)
	if len(f) < 8 {
		return Entry{}, fmt.Errorf("%s: expected 8 fields in %q", what, line)
	}
	mode, err := strconv.ParseUint(strings.TrimPrefix(f[0], "0x"), modeBase, 32)
	if err != nil {
		return Entry{}, fmt.Errorf("%s: bad mode %q", what, f[0])
	}
	var e Entry
	if e.Size, err = parseInt("size", f[1]); err != nil {
		return Entry{}, err
	}
	if e.MTime, err = parseEpoch(f[2]); err != nil {
		return Entry{}, err
	}
	if e.ATime, err = parseEpoch(f[3]); err != nil {
		return Entry{}, err
	}
	if e.CTime, err = parseEpoch(f[4]); err != nil {
		return Entry{}, err
	}
	uid, err := parseInt("uid", f[5])
	if err != nil {
		return Entry{}, err
	}
	gid, err := parseInt("gid", f[6])
	if err != nil {
		return Entry{}, err
	}
	e.Mode = uint32(mode)
	e.Uid = int(uid)
	e.Gid = int(gid)
	name := strings.TrimRight(f[7], "/")
	if name == "" {
		name = "/"
	} else if !keepPath {
		name = path.Base(name)
	}
	e.Name = name
	return e, nil
}

// ParseListing turns the payload of the enum, info and linfo commands into
// entries. The first line names the backend that produced the rest.
func ParseListing(lines []string) (string, []Entry, error) {
	return parseListing(lines, false)
}

// ParseFoundListing does the same for the answer of a tree search, where
// Entry.Name carries the full path of the hit rather than its name.
func ParseFoundListing(lines []string) (string, []Entry, error) {
	return parseListing(lines, true)
}

func parseListing(lines []string, keepPath bool) (string, []Entry, error) {
	if len(lines) == 0 {
		return "", nil, fmt.Errorf("fishplus: empty listing")
	}
	mode := strings.TrimSpace(strings.TrimPrefix(lines[0], "M "))
	if !strings.HasPrefix(lines[0], "M ") || mode == "" {
		return "", nil, fmt.Errorf("fishplus: listing without a mode marker: %q", lines[0])
	}
	// The ls backend carries its time dialect in the marker, because the two
	// print timestamps differently and nothing else in the line says which.
	mode, variant, _ := strings.Cut(mode, " ")
	var parse func(string) (Entry, error)
	switch mode {
	case "find":
		parse = parseFindEntry
	case "stat":
		parse = func(l string) (Entry, error) { return parseStatLike(l, 16, "stat listing", keepPath) }
	case "statbsd":
		parse = func(l string) (Entry, error) { return parseStatLike(l, 8, "bsd stat listing", keepPath) }
	case "ls":
		parse = func(l string) (Entry, error) { return parseLsEntry(l, variant, keepPath) }
	default:
		return mode, nil, fmt.Errorf("fishplus: unknown listing mode %q", mode)
	}
	entries := make([]Entry, 0, len(lines)-1)
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		e, err := parse(line)
		if err != nil {
			// A stray diagnostic line must not cost the user the panel.
			continue
		}
		if e.Name == "." || e.Name == ".." || e.Name == "" {
			continue
		}
		entries = append(entries, e)
	}
	return mode, entries, nil
}

// Client is the file system view of a FISH+ session.
type Client struct {
	sess *Session

	mu        sync.Mutex
	writeMode string
}

// NewClient wraps a session that has already completed its handshake. The
// write backend is remembered here rather than looked up per request,
// because wmode can change it at runtime and the client has to encode its
// payload the way the current backend expects it.
func NewClient(sess *Session) *Client {
	return &Client{sess: sess, writeMode: sess.Features().WriteMode()}
}

// Session exposes the underlying protocol session.
func (c *Client) Session() *Session { return c.sess }

// Enum lists a directory. Entries come in whatever order the remote tool
// produced them; "." and ".." are dropped.
func (c *Client) Enum(ctx context.Context, dir string) ([]Entry, error) {
	resp, err := c.sess.ExecPath(ctx, "enum", dir)
	if err != nil {
		return nil, err
	}
	if err := resp.Err("enum " + dir); err != nil {
		return nil, err
	}
	_, entries, err := ParseListing(resp.Lines)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// TargetDirs reports, in the same order, whether each path resolves to a
// directory. All paths travel in one protocol request: callers use this for
// symlinks returned by a directory listing, where one Stat round trip per
// link would dominate the listing itself. A missing, broken or inaccessible
// target is reported as false, matching the shell's -d predicate.
func (c *Client) TargetDirs(ctx context.Context, paths []string) ([]bool, error) {
	if len(paths) == 0 {
		return []bool{}, nil
	}
	resp, err := c.sess.ExecPaths(ctx, "isdirs", paths, strconv.Itoa(len(paths)))
	if err != nil {
		return nil, err
	}
	if err := resp.Err("isdirs"); err != nil {
		return nil, err
	}
	if len(resp.Lines) != len(paths) {
		return nil, fmt.Errorf("fishplus: isdirs returned %d answers for %d paths", len(resp.Lines), len(paths))
	}
	out := make([]bool, len(paths))
	for i, line := range resp.Lines {
		switch line {
		case "0":
		case "1":
			out[i] = true
		default:
			return nil, fmt.Errorf("fishplus: isdirs returned invalid answer %q for path %d", line, i)
		}
	}
	return out, nil
}

// Stat resolves symlinks, Lstat reports the link itself.
func (c *Client) Stat(ctx context.Context, p string) (Entry, error) {
	return c.info(ctx, "info", p)
}

func (c *Client) Lstat(ctx context.Context, p string) (Entry, error) {
	return c.info(ctx, "linfo", p)
}

func (c *Client) info(ctx context.Context, cmd, p string) (Entry, error) {
	resp, err := c.sess.ExecPath(ctx, cmd, p)
	if err != nil {
		return Entry{}, err
	}
	if err := resp.Err(cmd + " " + p); err != nil {
		return Entry{}, err
	}
	_, entries, err := ParseListing(resp.Lines)
	if err != nil {
		return Entry{}, err
	}
	if len(entries) != 1 {
		return Entry{}, fmt.Errorf("fishplus: %s %q returned %d entries", cmd, p, len(entries))
	}
	e := entries[0]
	// The backends disagree on whether they echo the full path or the base
	// name, so the name is taken from the request instead.
	e.Name = path.Base(p)
	return e, nil
}

// ReadLink returns the target of a symlink exactly as stored on the host.
func (c *Client) ReadLink(ctx context.Context, p string) (string, error) {
	resp, err := c.sess.ExecPath(ctx, "rdlink", p)
	if err != nil {
		return "", err
	}
	if err := resp.Err("rdlink " + p); err != nil {
		return "", err
	}
	return strings.Join(resp.Lines, "\n"), nil
}

// Pwd returns the directory the remote shell started in. That is where a
// panel opens first, the same way an interactive login lands in a home
// directory rather than at the root.
func (c *Client) Pwd(ctx context.Context) (string, error) {
	resp, err := c.sess.Exec(ctx, "pwd")
	if err != nil {
		return "", err
	}
	if err := resp.Err("pwd"); err != nil {
		return "", err
	}
	return strings.Join(resp.Lines, "\n"), nil
}

// SetListingMode forces a metadata backend instead of the one picked during
// the handshake. Meant for tests and for troubleshooting an odd host.
func (c *Client) SetListingMode(ctx context.Context, mode string) error {
	resp, err := c.sess.Exec(ctx, "mode", mode)
	if err != nil {
		return err
	}
	return resp.Err("mode " + mode)
}
