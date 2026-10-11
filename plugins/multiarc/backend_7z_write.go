package multiarc

import (
	"context"
	"errors"
	"strings"
)

// errNoSevenZip is what every 7z write answers when no 7z, 7za or 7zr is on
// PATH any more (it was when the archive was opened).
var errNoSevenZip = errors.New("multiarc: changing a .7z archive needs 7z, 7za or 7zr on PATH")

// checkWrite: 7-Zip can add, replace and delete in every build of it, 7zr
// included (7zr only drops the other formats, and this backend only ever
// handles .7z).
func (b sevenZipBackend) checkWrite(context.Context, string, writeOp, string) error {
	if _, ok := b.tool(); !ok {
		return errNoSevenZip
	}
	return nil
}

// sevenZipNameSwitches is the switch list that goes before "--" in a 7z
// command naming members: -spd turns off wildcard matching, needed only for
// a name that holds "*" or "?" and left out otherwise so that an old p7zip
// without that switch still takes every ordinary name. "--" after it stops
// 7-Zip's parsing of switches, so a member called "-x" is just a name; it
// does not stop 7-Zip reading an "@" name as a listfile (see
// sevenZipMemberArgs, which is what actually protects those).
func sevenZipNameSwitches(names []string, switches ...string) []string {
	if hasWildcard(names) {
		switches = append(switches, "-spd")
	}
	return switches
}

// sevenZipMemberArgs rewrites a member name starting with "@" to "./name".
// 7-Zip reads an "@" argument as the name of a listfile to read further
// names from -- on the a, d and x commands alike, and "--" does not turn
// this off the way it does switch parsing -- so a member actually called
// "@name" would otherwise make 7-Zip fail with "Cannot find listfile" (or
// worse, read some unrelated file as one). Naming it "./@name" instead
// reaches the same file without the argument starting with "@"; the
// listing then has the "./" stripped back off (see sevenZipEntry).
func sevenZipMemberArgs(names []string) []string {
	out := make([]string, len(names))
	for i, m := range names {
		if strings.HasPrefix(m, "@") {
			m = "./" + m
		}
		out[i] = m
	}
	return out
}

// add runs "7z a" in stageDir, so each member is stored under the relative
// path it was staged at. 7z a replaces a member of the same name whatever
// the two timestamps say, so replaced needs no work of its own. 7-Zip
// builds the updated archive in a temporary file and renames it over the
// original only once it is complete.
func (b sevenZipBackend) add(ctx context.Context, localPath, stageDir string, members, _ []string) error {
	bin, ok := b.tool()
	if !ok {
		return errNoSevenZip
	}
	head := append([]string{"a"}, sevenZipNameSwitches(members, "-t7z", "-y")...)
	head = append(head, localPath)
	return runChunked(ctx, stageDir, bin, head, sevenZipMemberArgs(members))
}

// remove runs "7z d". Naming a directory deletes everything under it too,
// so only the covering names are passed.
func (b sevenZipBackend) remove(ctx context.Context, localPath string, raws []string) error {
	bin, ok := b.tool()
	if !ok {
		return errNoSevenZip
	}
	names := coveringNames(raws)
	head := append([]string{"d"}, sevenZipNameSwitches(names, "-y")...)
	head = append(head, localPath)
	return runChunked(ctx, "", bin, head, sevenZipMemberArgs(names))
}
