package multiarc

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// zipBackend wraps Info-ZIP's unzip. "-Z1" asks for its zipinfo mode's
// names-only listing (one path per line, directories carrying their own
// trailing "/" entry the way zip already stores them), which is the same
// bare format tarBackend prefers and for the same reason: it needs no
// column parsing that a space in a filename could throw off.
type zipBackend struct{}

func (zipBackend) id() string { return "zip" }

func zipAvailable() bool { return toolAvailable("unzip") }

func (zipBackend) list(ctx context.Context, localPath string) ([]entry, error) {
	out, errOut, err := runTool(ctx, "unzip", "-Z1", localPath)
	if err != nil {
		if isEmptyZipListing(out, errOut) {
			return nil, nil
		}
		return nil, fmt.Errorf("multiarc: unzip -Z1 %s: %w (%s)", localPath, err, strings.TrimSpace(string(errOut)))
	}
	return parseBareNameListing(out), nil
}

// isEmptyZipListing recognizes how unzip answers for a zip with no members
// at all: exit status 1 and the one line "Empty zipfile." on stdout, where a
// damaged archive would have explained itself on stderr instead. Such a zip
// is also what "zip -d" leaves behind once its last member is deleted, so it
// has to list as empty rather than fail.
func isEmptyZipListing(stdout, stderr []byte) bool {
	return strings.TrimSpace(string(stdout)) == "Empty zipfile." && strings.TrimSpace(string(stderr)) == ""
}

func (zipBackend) extractAll(ctx context.Context, localPath, destDir string) error {
	_, errOut, err := runTool(ctx, "unzip", "-o", "-q", localPath, "-d", destDir)
	if err != nil {
		return fmt.Errorf("multiarc: unzip -o -q %s -d %s: %w (%s)", localPath, destDir, err, strings.TrimSpace(string(errOut)))
	}
	return nil
}

func (zipBackend) extractOne(ctx context.Context, localPath, destDir, member string) error {
	if member == "" {
		return errors.New("multiarc: extractOne needs a member path")
	}
	pattern := unzipLiteral(member)
	_, errOut, err := runTool(ctx, "unzip", "-o", "-q", localPath, pattern, "-d", destDir)
	if err != nil {
		return fmt.Errorf("multiarc: unzip -o -q %s %s -d %s: %w (%s)", localPath, pattern, destDir, err, strings.TrimSpace(string(errOut)))
	}
	return nil
}

// unzipLiteral turns a member name into an unzip pattern matching exactly
// that name. unzip reads every member argument as a wildcard pattern and
// has no "--" and no switch to turn that off, so "dir/[ab].txt" would
// extract dir/a.txt and dir/b.txt instead, and "-x.txt" would be taken for
// options. Each special character goes in a one-character class of its
// own -- "[*]", "[?]", "[[]", and "[-]" for a leading dash -- and a
// backslash, unzip's escape character, is doubled.
func unzipLiteral(member string) string {
	var b strings.Builder
	for i, r := range member {
		switch {
		case r == '*' || r == '?' || r == '[' || (r == '-' && i == 0):
			b.WriteByte('[')
			b.WriteRune(r)
			b.WriteByte(']')
		case r == '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
