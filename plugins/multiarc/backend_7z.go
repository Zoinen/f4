package multiarc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// sevenZipBackend wraps whichever of 7z, 7za or 7zr is on PATH (checked in
// that order by sevenZipTool, the same preference far2l's own 7z wrapper
// uses: the full 7z binary first, then the smaller standalone builds).
type sevenZipBackend struct{ bin string }

func (b sevenZipBackend) id() string { return "7z" }

// sevenZipTool reports the first of the three binaries found on PATH.
func sevenZipTool() (string, bool) {
	for _, name := range []string{"7z", "7za", "7zr"} {
		if toolAvailable(name) {
			return name, true
		}
	}
	return "", false
}

// tool returns the binary detectFormat resolved for this backend instance,
// re-probing PATH only for a zero-value sevenZipBackend (e.g. one built
// directly in a test rather than through detectFormat).
func (b sevenZipBackend) tool() (string, bool) {
	if b.bin != "" {
		return b.bin, true
	}
	return sevenZipTool()
}

// list parses "7z l -slt", the "show technical information" listing: one
// blank-line-separated block of "Key = Value" lines per archive member,
// plus one such block up front describing the archive itself. That header
// block is what a plain columnar "7z l" listing does not offer a stable
// alternative to -- the column widths and even which columns appear vary
// by 7-Zip version and locale, while every -slt block, header or member,
// is the same "Key = Value" shape regardless of version. The header is
// told apart from a member by its own "Type" key, which no member block
// carries.
func (b sevenZipBackend) list(ctx context.Context, localPath string) ([]entry, error) {
	bin, ok := b.tool()
	if !ok {
		return nil, errors.New("multiarc: no 7z/7za/7zr on PATH")
	}
	out, errOut, err := runTool(ctx, bin, "l", "-slt", localPath)
	if err != nil {
		return nil, fmt.Errorf("multiarc: %s l -slt %s: %w (%s)", bin, localPath, err, strings.TrimSpace(string(errOut)))
	}
	return parseSevenZipListing(out), nil
}

func parseSevenZipListing(out []byte) []entry {
	var entries []entry
	block := map[string]string{}
	flush := func() {
		if len(block) == 0 {
			return
		}
		if _, isHeader := block["Type"]; !isHeader {
			if p, ok := block["Path"]; ok && p != "" {
				entries = append(entries, sevenZipEntry(block, p))
			}
		}
		block = map[string]string{}
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		block[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	flush()
	return entries
}

func sevenZipEntry(block map[string]string, raw string) entry {
	p := strings.ReplaceAll(strings.Trim(raw, "/"), "\\", "/")
	// A member added as "./@name" (see sevenZipMemberArgs) keeps that
	// leading "./" in the listing; stripped here the same way
	// parseBareNameListing strips it for tar and zip, so the VFS path is
	// just "@name" whichever way 7-Zip reports it.
	p = strings.TrimPrefix(p, "./")
	e := entry{Path: p, Raw: raw}
	attr := block["Attributes"]
	e.IsDir = block["Folder"] == "+" || strings.HasPrefix(attr, "D")
	if size, err := strconv.ParseInt(block["Size"], 10, 64); err == nil {
		e.Size = size
		e.SizeKnown = true
	}
	if mtime, err := time.Parse("2006-01-02 15:04:05", block["Modified"]); err == nil {
		e.MTime = mtime
	}
	return e
}

func (b sevenZipBackend) extractAll(ctx context.Context, localPath, destDir string) error {
	bin, ok := b.tool()
	if !ok {
		return errors.New("multiarc: no 7z/7za/7zr on PATH")
	}
	_, errOut, err := runTool(ctx, bin, "x", "-y", "-o"+destDir, localPath)
	if err != nil {
		return fmt.Errorf("multiarc: %s x -y -o%s %s: %w (%s)", bin, destDir, localPath, err, strings.TrimSpace(string(errOut)))
	}
	return nil
}

func (b sevenZipBackend) extractOne(ctx context.Context, localPath, destDir, member string) error {
	if member == "" {
		return errors.New("multiarc: extractOne needs a member path")
	}
	bin, ok := b.tool()
	if !ok {
		return errors.New("multiarc: no 7z/7za/7zr on PATH")
	}
	// extract asks for a name the listing already gave us verbatim (member
	// came from the archive's own raw name, via rawName/Raw), which rules
	// out sevenZipMemberArgs' "./" rewrite here: 7-Zip normalizes away the
	// "./" an add was given before deciding the stored name, so asking to
	// extract "./name" for one actually stored as "name" silently extracts
	// nothing -- this function tried that once and it broke every
	// platform's reading back of a member added through this same package.
	//
	// "@name" still needs its own answer, though: 7-Zip reads an "@"
	// filename argument as a listfile wherever it falls, "--" or not, on
	// "x" the same as "a" (confirmed on macOS's p7zip: extracting a real
	// "@at.txt" this way failed with "Cannot find listfile"). "-i!name"
	// names it as an include pattern instead, which is not that argument
	// position and so is never read as one, without changing what name is
	// asked for the way "./name" would.
	if strings.HasPrefix(member, "@") {
		args := []string{"x", "-y", "-o" + destDir, "-i!" + member, localPath}
		_, errOut, err := runTool(ctx, bin, args...)
		if err != nil {
			return toolFailure(bin, args, err, errOut)
		}
		return nil
	}
	// "--" keeps a member called "-x" from being read as a switch; -spd
	// (see sevenZipNameSwitches) keeps one holding "*" or "?" literal.
	args := append([]string{"x"}, sevenZipNameSwitches([]string{member}, "-y", "-o"+destDir)...)
	args = append(args, localPath, "--", member)
	_, errOut, err := runTool(ctx, bin, args...)
	if err != nil {
		return toolFailure(bin, args, err, errOut)
	}
	return nil
}
