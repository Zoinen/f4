package intchecker

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// maxChecksumFileSize bounds how much of a checksum file is read. Even a
// SHA-512 list of a million files stays well below it; anything larger is not
// a checksum file.
const maxChecksumFileSize = 64 << 20

var (
	// errNoChecksums means the file held no line that looks like a checksum.
	errNoChecksums = errors.New("no checksums found")
	// errUnknownHashLength means the digests match no supported algorithm.
	errUnknownHashLength = errors.New("unknown hash length")
	// errChecksumFileTooLarge means the file is too big to be a checksum list.
	errChecksumFileTooLarge = errors.New("checksum file is too large")
	// errAllMissing means none of the listed files exists in the directory
	// that was checked; the user is asked for another directory.
	errAllMissing = errors.New("none of the listed files was found")
)

// ChecksumFile is a parsed checksum file.
type ChecksumFile struct {
	Algorithm Algorithm
	Entries   []Entry
	// Malformed counts the lines that are neither blank, a comment nor a
	// checksum of the file's algorithm.
	Malformed int
}

// algorithmForExtension picks the algorithm from a checksum file name.
func algorithmForExtension(name string) (Algorithm, bool) {
	lower := strings.ToLower(name)
	for i, info := range algorithms {
		if strings.HasSuffix(lower, info.ext) && len(lower) > len(info.ext) {
			return Algorithm(i), true
		}
	}
	return 0, false
}

// isChecksumFileName reports whether name has a checksum file extension.
func isChecksumFileName(name string) bool {
	_, ok := algorithmForExtension(name)
	return ok
}

// algorithmForDigestSize picks the algorithm whose digest has size bytes.
// SHA-384 and SHA-512 differ in length, so every size is unambiguous.
func algorithmForDigestSize(size int) (Algorithm, bool) {
	for i := range algorithms {
		if Algorithm(i).New().Size() == size {
			return Algorithm(i), true
		}
	}
	return 0, false
}

// ParseHashFile reads a checksum file named name. It understands the lines
// FormatHashFile writes and what other tools write: "<hex> *<name>" (binary
// mode), "<hex>  <name>" (text mode), "<hex> <name>", the GNU escaped form
// with a leading backslash, and SFV "<name> <CRC32>". Blank lines and lines
// starting with ';' or '#' are comments.
//
// The algorithm comes from the extension; when there is none or the digests
// do not have its length (a mislabelled file), it comes from the length of the
// first digest. Lines of another length are counted as malformed.
func ParseHashFile(name string, data []byte) (ChecksumFile, error) {
	var res ChecksumFile
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	extAlg, extOK := algorithmForExtension(name)
	sfvFirst := extOK && extAlg == AlgCRC32

	var entries []Entry
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || trimmed[0] == ';' || trimmed[0] == '#' {
			continue
		}
		var (
			entry Entry
			ok    bool
		)
		if sfvFirst {
			if entry, ok = parseSFVLine(line); !ok {
				entry, ok = parseCoreutilsLine(line)
			}
		} else if entry, ok = parseCoreutilsLine(line); !ok {
			entry, ok = parseSFVLine(line)
		}
		if !ok {
			res.Malformed++
			continue
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return res, errNoChecksums
	}

	alg := extAlg
	if !extOK || len(entries[0].Sum) != extAlg.New().Size() {
		var ok bool
		if alg, ok = algorithmForDigestSize(len(entries[0].Sum)); !ok {
			return res, errUnknownHashLength
		}
	}
	size := alg.New().Size()
	for _, e := range entries {
		if len(e.Sum) != size {
			res.Malformed++
			continue
		}
		res.Entries = append(res.Entries, e)
	}
	res.Algorithm = alg
	return res, nil
}

// parseCoreutilsLine reads "<hex> *<name>", "<hex>  <name>" or "<hex> <name>",
// optionally prefixed with the backslash that marks an escaped name.
func parseCoreutilsLine(line string) (Entry, bool) {
	escaped := strings.HasPrefix(line, `\`)
	if escaped {
		line = line[1:]
	}
	sep := strings.IndexByte(line, ' ')
	if sep <= 0 {
		return Entry{}, false
	}
	sum, err := hex.DecodeString(line[:sep])
	if err != nil {
		return Entry{}, false
	}
	name := line[sep+1:]
	if strings.HasPrefix(name, "*") || strings.HasPrefix(name, " ") {
		name = name[1:]
	}
	if escaped {
		var ok bool
		if name, ok = unescapeCoreutilsName(name); !ok {
			return Entry{}, false
		}
	}
	if name == "" {
		return Entry{}, false
	}
	return Entry{Name: name, Sum: sum}, true
}

// unescapeCoreutilsName undoes escapeCoreutilsName.
func unescapeCoreutilsName(name string) (string, bool) {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i == len(name) {
			return "", false
		}
		switch name[i] {
		case '\\':
			b.WriteByte('\\')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		default:
			return "", false
		}
	}
	return b.String(), true
}

// parseSFVLine reads "<name> <CRC32>"; the name may contain spaces.
func parseSFVLine(line string) (Entry, bool) {
	line = strings.TrimRight(line, " \t")
	sep := strings.LastIndexAny(line, " \t")
	if sep <= 0 {
		return Entry{}, false
	}
	crc := line[sep+1:]
	if len(crc) != 8 {
		return Entry{}, false
	}
	sum, err := hex.DecodeString(crc)
	if err != nil {
		return Entry{}, false
	}
	name := strings.TrimRight(line[:sep], " \t")
	if name == "" {
		return Entry{}, false
	}
	return Entry{Name: name, Sum: sum}, true
}

// readChecksumFile loads a checksum file through fs.
func readChecksumFile(ctx context.Context, fs vfs.VFS, path string) (data []byte, err error) {
	f, err := fs.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	var b bytes.Buffer
	buf := make([]byte, 64<<10)
	for {
		n, readErr := f.Read(ctx, buf)
		b.Write(buf[:n])
		if b.Len() > maxChecksumFileSize {
			return nil, errChecksumFileTooLarge
		}
		if readErr == io.EOF {
			return b.Bytes(), nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}

// entryStatus is the verdict on one listed file.
type entryStatus int

const (
	statusOK entryStatus = iota
	statusMismatch
	statusMissing
	statusReadError
	statusCount
)

// entryResult is the verdict on one listed file; Err is set for a read error.
type entryResult struct {
	Name   string
	Status entryStatus
	Err    error
}

// validateJob describes one "Validate files" run: check the entries of file
// against the files in dir on fs.
type validateJob struct {
	fs       vfs.VFS
	hashPath string // the checksum file, for messages
	dir      string // where the listed relative names are looked up
	file     ChecksumFile
	encoding fileEncoding // what the file was read as, for the dialog
	// ignoreMissing and stopOnMismatch are the "Validate files" dialog's
	// checkboxes (f4#1623 point 2), carried from validateOptions.
	ignoreMissing  bool
	stopOnMismatch bool
}

// validateResult is what a run found, in checksum file order.
type validateResult struct {
	Results []entryResult
	Counts  [statusCount]int
	// StoppedEarly is set when stopOnMismatch cut the run short, right after
	// the first mismatch: Results then covers only a prefix of file.Entries.
	StoppedEarly bool
}

func (r *validateResult) add(name string, status entryStatus, err error) {
	r.Results = append(r.Results, entryResult{Name: name, Status: status, Err: err})
	r.Counts[status]++
}

// resolveEntry finds a listed file under dir. Names are relative to dir unless
// absolute; '/' separates directories. A name with backslashes, as Windows
// tools write them, is retried with '/' when it is not found as is.
func resolveEntry(ctx context.Context, fs vfs.VFS, dir, name string) (string, vfs.VFSItem, error) {
	path := entryPath(fs, dir, name)
	item, err := fs.Stat(ctx, path)
	if err != nil && strings.Contains(name, `\`) && ctx.Err() == nil {
		alt := entryPath(fs, dir, strings.ReplaceAll(name, `\`, "/"))
		if altItem, altErr := fs.Stat(ctx, alt); altErr == nil {
			return alt, altItem, nil
		}
	}
	return path, item, err
}

func entryPath(fs vfs.VFS, dir, name string) string {
	if fs.IsAbs(name) {
		return name
	}
	return fs.Join(append([]string{dir}, strings.Split(name, "/")...)...)
}

// runValidate checks every listed file. It first looks the files up, so the
// progress knows the total size and a wrong directory is noticed before
// anything is read: when not a single file is there it returns errAllMissing.
// A cancelled run returns ctx.Err().
func runValidate(ctx context.Context, job validateJob, reporter progressReporter) (validateResult, error) {
	var res validateResult
	entries := job.file.Entries

	type found struct {
		entry Entry
		path  string
		size  int64
	}
	var inputs []found
	var total int64
	missing := 0
	var located []entryResult // lookup verdicts, merged back in file order
	locating := vtui.Msg("IntChecker.ProgressLocating")
	for i, e := range entries {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		reporter.UpdateTransfer(locating, e.Name, 0, fmt.Sprintf(vtui.Msg("IntChecker.ProgressFiles"), i+1, len(entries)), percent(int64(i), int64(len(entries))), "")
		path, item, err := resolveEntry(ctx, job.fs, job.dir, e.Name)
		switch {
		case err != nil && ctx.Err() != nil:
			return res, ctx.Err()
		case err != nil && errors.Is(err, iofs.ErrNotExist):
			missing++
			located = append(located, entryResult{Name: e.Name, Status: statusMissing})
		case err != nil:
			located = append(located, entryResult{Name: e.Name, Status: statusReadError, Err: err})
		case item.IsDir:
			located = append(located, entryResult{Name: e.Name, Status: statusReadError, Err: errors.New(vtui.Msg("IntChecker.IsDirectory"))})
		default:
			located = append(located, entryResult{Name: e.Name, Status: statusOK})
			inputs = append(inputs, found{entry: e, path: path, size: item.Size})
			total += item.Size
		}
	}
	if missing == len(entries) {
		return res, errAllMissing
	}

	verifying := vtui.Msg("IntChecker.ProgressVerifying")
	var done int64
	buf := make([]byte, hashBufferSize)
	next := 0 // index into inputs, which follow located's statusOK rows
	speed := newProgressSpeed()
	for _, loc := range located {
		if loc.Status != statusOK {
			// f4#1623 point 2: with "Ignore missing files" on, a listed file
			// that is not on disk is left out of the report entirely, as if
			// it were never listed -- not counted, not shown as a problem.
			if loc.Status == statusMissing && job.ignoreMissing {
				continue
			}
			res.add(loc.Name, loc.Status, loc.Err)
			continue
		}
		in := inputs[next]
		next++
		report := func(fileDone int64) {
			totalText := fmt.Sprintf(vtui.Msg("IntChecker.ProgressFiles"), next, len(inputs))
			reporter.UpdateTransfer(verifying, in.entry.Name, percent(fileDone, in.size), totalText, percent(done+fileDone, total), speed.text(done+fileDone, total))
		}
		report(0)
		sum, read, err := hashFile(ctx, job.fs, in.path, job.file.Algorithm, buf, report)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return res, ctxErr
			}
			res.add(in.entry.Name, statusReadError, err)
			done += in.size
			continue
		}
		done += read
		if bytes.Equal(sum, in.entry.Sum) {
			res.add(in.entry.Name, statusOK, nil)
			continue
		}
		res.add(in.entry.Name, statusMismatch, nil)
		// f4#1623 point 2: with "Stop on first mismatch" on, the run ends
		// right here instead of checking the rest of the list.
		if job.stopOnMismatch {
			res.StoppedEarly = true
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return res, err
	}
	return res, nil
}

// statusLabelKeys are the message keys of the problem verdicts in a report.
var statusLabelKeys = [statusCount]string{
	statusMismatch:  "IntChecker.StatusMismatch",
	statusMissing:   "IntChecker.StatusMissing",
	statusReadError: "IntChecker.StatusReadError",
}

// validateReport is the text shown after a run: the counts, then the files
// that did not pass (up to shown of them), then the malformed line count.
func validateReport(res validateResult, malformed int) string {
	var lines []string
	checked := len(res.Results)
	if res.Counts[statusOK] == checked {
		lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.AllOK"), checked))
	} else {
		lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.ValidateSummary"), checked,
			res.Counts[statusOK], res.Counts[statusMismatch], res.Counts[statusMissing], res.Counts[statusReadError]))
		const shown = 10
		listed := 0
		problems := checked - res.Counts[statusOK]
		for _, r := range res.Results {
			if r.Status == statusOK {
				continue
			}
			if listed == 0 {
				lines = append(lines, "")
			}
			if listed == shown {
				lines = append(lines, fmt.Sprintf(vtui.Msg("IntChecker.MoreErrors"), problems-shown))
				break
			}
			line := fmt.Sprintf("%s: %s", vtui.Msg(statusLabelKeys[r.Status]), r.Name)
			if r.Err != nil {
				line += fmt.Sprintf(" (%v)", r.Err)
			}
			lines = append(lines, line)
			listed++
		}
	}
	if res.StoppedEarly {
		lines = append(lines, "", vtui.Msg("IntChecker.StoppedOnMismatch"))
	}
	if malformed > 0 {
		lines = append(lines, "", fmt.Sprintf(vtui.Msg("IntChecker.Malformed"), malformed))
	}
	return strings.Join(lines, "\n")
}
