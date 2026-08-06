package fishplus

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"
)

// The ls backend exists for hosts that have neither GNU find nor either
// stat: OpenWrt with BusyBox is the first one reported, and there will be
// others. ls is the one listing tool that is always there, and the one whose
// output was never meant to be parsed.
//
// Two things make it parseable at all. The timestamp is asked for in full,
// because the short form drops the year on anything older than six months
// and there is no way to guess it back. And the ids are asked for
// numerically, because a user name can contain a space and would then be
// indistinguishable from the column after it. What is left is a fixed number
// of fields and then the name, however many spaces it has in it.

// lsFieldCount is how many fields come before the name in each dialect:
// mode, links, uid, gid, size, and then the timestamp, which is one field as
// an epoch, three as a full iso date and four in the BSD form.
func lsFieldCount(variant string) int {
	switch variant {
	case "bsd":
		return 9
	case "iso":
		// date, time and zone, whether or not the time carries a fraction.
		return 8
	default:
		return 6
	}
}

// parseLsEntry reads one line of ls -lan output. keepPath has the same
// meaning as for the stat backends: a listing wants the bare name, a search
// wants the path it was handed.
func parseLsEntry(line, variant string, keepPath bool) (Entry, error) {
	if strings.HasPrefix(line, "total ") {
		return Entry{}, fmt.Errorf("fishplus: ls listing: the total line")
	}
	n := lsFieldCount(variant)
	fields := strings.Fields(line)
	if len(fields) <= n {
		return Entry{}, fmt.Errorf("fishplus: ls listing: %q has %d fields", line, len(fields))
	}

	mode, err := parseLsMode(fields[0])
	if err != nil {
		return Entry{}, err
	}
	uid, err1 := strconv.Atoi(fields[2])
	gid, err2 := strconv.Atoi(fields[3])
	size, err3 := strconv.ParseInt(fields[4], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return Entry{}, fmt.Errorf("fishplus: ls listing: bad numbers in %q", line)
	}
	when, err := parseLsTime(fields[5:n], variant)
	if err != nil {
		return Entry{}, err
	}

	// Everything after the fixed fields is the name, spaces and all. Finding
	// it by index rather than by rejoining the fields keeps runs of spaces
	// in a name intact.
	name := lsNameOf(line, fields, n)
	if name == "" {
		return Entry{}, fmt.Errorf("fishplus: ls listing: no name in %q", line)
	}
	// A symlink is printed with its target, which is not part of the name.
	if mode&0170000 == 0120000 {
		if at := strings.Index(name, " -> "); at >= 0 {
			name = name[:at]
		}
	}
	name = strings.TrimRight(name, "/")
	if name == "" {
		name = "/"
	} else if !keepPath {
		name = path.Base(name)
	}

	return Entry{
		Name:  name,
		Size:  size,
		Mode:  mode,
		MTime: when,
		ATime: when,
		CTime: when,
		Uid:   uid,
		Gid:   gid,
	}, nil
}

// lsNameOf finds where the name starts by walking past the fixed fields in
// the original line, so that a name made of several spaces survives.
func lsNameOf(line string, fields []string, n int) string {
	at := 0
	for i := 0; i < n; i++ {
		idx := strings.Index(line[at:], fields[i])
		if idx < 0 {
			return ""
		}
		at += idx + len(fields[i])
	}
	// Exactly one space separates the last fixed column from the name: ls
	// pads inside a column, never between them. Trimming everything here
	// would eat the leading spaces of a name that begins with them.
	rest := line[at:]
	if len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	return rest
}

// parseLsTime reads the timestamp of whichever dialect produced it. The BSD
// form is the only one without a zone in it, so it is read in the local zone
// here: a host in another zone will be off by the difference, which is the
// price of a format that does not carry one.
func parseLsTime(fields []string, variant string) (time.Time, error) {
	switch variant {
	case "iso":
		if len(fields) != 3 {
			return time.Time{}, fmt.Errorf("fishplus: ls listing: bad timestamp %v", fields)
		}
		// A fraction may or may not be there; the layout takes both.
		t, err := time.Parse("2006-01-02 15:04:05.999999999 -0700", strings.Join(fields, " "))
		if err != nil {
			return time.Time{}, fmt.Errorf("fishplus: ls listing: bad timestamp %v", fields)
		}
		return t, nil
	case "bsd":
		if len(fields) != 4 {
			return time.Time{}, fmt.Errorf("fishplus: ls listing: bad timestamp %v", fields)
		}
		t, err := time.ParseInLocation("Jan 2 15:04:05 2006", strings.Join(fields, " "), time.Local)
		if err != nil {
			return time.Time{}, fmt.Errorf("fishplus: ls listing: bad timestamp %v", fields)
		}
		return t, nil
	}
	if len(fields) != 1 {
		return time.Time{}, fmt.Errorf("fishplus: ls listing: bad timestamp %v", fields)
	}
	secs, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("fishplus: ls listing: bad timestamp %q", fields[0])
	}
	return time.Unix(secs, 0), nil
}

// parseLsMode turns "drwxr-sr-t" into the mode bits a stat would have
// reported. A trailing marker for extended attributes or an ACL, which macOS
// and Linux both add, is ignored.
func parseLsMode(s string) (uint32, error) {
	if len(s) < 10 {
		return 0, fmt.Errorf("fishplus: ls listing: bad mode %q", s)
	}
	var mode uint32
	switch s[0] {
	case 'd':
		mode = 0040000
	case 'l':
		mode = 0120000
	case '-':
		mode = 0100000
	case 'c':
		mode = 0020000
	case 'b':
		mode = 0060000
	case 'p':
		mode = 0010000
	case 's':
		mode = 0140000
	default:
		return 0, fmt.Errorf("fishplus: ls listing: unknown type %q", s[:1])
	}
	// Each triple is rwx, with the execute position doubling as the setuid,
	// setgid and sticky flag: a lower case letter means the flag and the
	// execute bit, an upper case one means the flag alone.
	for triple := 0; triple < 3; triple++ {
		shift := uint(6 - 3*triple)
		if s[1+3*triple] == 'r' {
			mode |= 4 << shift
		}
		if s[2+3*triple] == 'w' {
			mode |= 2 << shift
		}
		switch s[3+3*triple] {
		case 'x':
			mode |= 1 << shift
		case 's', 't':
			mode |= 1 << shift
			mode |= specialBit(triple)
		case 'S', 'T':
			mode |= specialBit(triple)
		}
	}
	return mode, nil
}

func specialBit(triple int) uint32 {
	switch triple {
	case 0:
		return 04000
	case 1:
		return 02000
	default:
		return 01000
	}
}
