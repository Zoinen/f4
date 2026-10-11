package multiarc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// writeOp names the kind of change a backend is asked to make, so that
// checkWrite can refuse the ones its tool cannot do before the user's data
// is staged anywhere.
type writeOp int

const (
	writeAdd     writeOp = iota // a file member that does not exist yet
	writeReplace                // a file member that exists and is overwritten
	writeMkDir                  // a new, empty directory member
	writeRemove                 // a member, or a directory with everything under it
)

// archiveWriter is the optional half of a backend that changes an archive
// in place, far2l multiarc's "add" and "delete" commands. Every write goes
// through a console archiver the same way reading does, and every one of
// them leaves the original archive untouched until the tool has finished:
// zip and 7z build the new archive beside the old one and rename it over,
// and the tar and gzip backends do the same thing themselves (see
// replaceArchive). A failed or canceled write therefore loses nothing.
type archiveWriter interface {
	// checkWrite reports whether op on member can be carried out with the
	// tools on PATH right now. Its error names the tool that is missing, or
	// says why the format or the tool cannot do it at all.
	checkWrite(ctx context.Context, localPath string, op writeOp, member string) error
	// add stores members -- slash-separated paths relative to stageDir,
	// each already staged there, a directory as an empty directory -- in
	// the archive at localPath. replaced holds the raw names of the members
	// this add overwrites, empty for a new one: a tool that appends rather
	// than replaces has to delete them first.
	add(ctx context.Context, localPath, stageDir string, members, replaced []string) error
	// remove deletes every member named in raws: the raw names of all the
	// entries under the directory (or of the one file) being deleted, the
	// directory's own entry included when it has one.
	remove(ctx context.Context, localPath string, raws []string) error
}

// toolFailure formats a failed archiver run the way every backend in this
// package reports one: the command, the error, and what the tool said.
func toolFailure(bin string, args []string, err error, stderr []byte) error {
	return fmt.Errorf("multiarc: %s %s: %w (%s)", bin, strings.Join(args, " "), err, strings.TrimSpace(string(stderr)))
}

// runToolChecked runs bin in dir and turns a failure into toolFailure.
func runToolChecked(ctx context.Context, dir, bin string, args ...string) error {
	_, stderr, err := runToolIn(ctx, dir, bin, args...)
	if err != nil {
		return toolFailure(bin, args, err, stderr)
	}
	return nil
}

// Command lines are split into chunks well below Windows' 32767-character
// limit on a whole command line, the smallest any supported platform has.
const (
	maxChunkBytes = 16 << 10
	maxChunkNames = 512
)

// chunkNames splits names into batches that keep one command line within
// maxChunkBytes and maxChunkNames. Deleting a directory with thousands of
// members from a zip is one zip -d per batch rather than one command line
// the operating system refuses to start.
func chunkNames(names []string) [][]string {
	var chunks [][]string
	var cur []string
	size := 0
	for _, name := range names {
		if len(cur) > 0 && (size+len(name)+1 > maxChunkBytes || len(cur) >= maxChunkNames) {
			chunks = append(chunks, cur)
			cur, size = nil, 0
		}
		cur = append(cur, name)
		size += len(name) + 1
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// runChunked runs "bin head... -- chunk..." in dir once per chunk of names.
func runChunked(ctx context.Context, dir, bin string, head, names []string) error {
	for _, chunk := range chunkNames(names) {
		args := append(append(append([]string(nil), head...), "--"), chunk...)
		if err := runToolChecked(ctx, dir, bin, args...); err != nil {
			return err
		}
	}
	return nil
}

// coveringNames returns the fewest of raws that still reach every one of
// them, for tools that delete a directory member together with everything
// under it (7z, GNU tar): a name is dropped when a kept one is its parent
// directory. For GNU tar this is required, not just shorter: a name whose
// members an earlier name on the same command line already deleted is
// reported as "Not found in archive", and tar fails.
func coveringNames(raws []string) []string {
	sorted := append([]string(nil), raws...)
	// A directory sorts before everything under it: it is a proper prefix
	// of each of their names.
	sort.Strings(sorted)
	kept := make([]string, 0, len(sorted))
	keptDirs := map[string]bool{}
	for _, raw := range sorted {
		name := strings.TrimSuffix(raw, "/")
		covered := keptDirs[name]
		for i := 0; i < len(name) && !covered; i++ {
			if name[i] == '/' && keptDirs[name[:i]] {
				covered = true
			}
		}
		if covered {
			continue
		}
		kept = append(kept, raw)
		keptDirs[name] = true
	}
	return kept
}

// hasWildcard reports whether any name holds a character 7-Zip would treat
// as a wildcard. 7z is only passed -spd ("no wildcards") then, so that an
// old p7zip without that switch still handles every ordinary name.
func hasWildcard(names []string) bool {
	for _, name := range names {
		if strings.ContainsAny(name, "*?") {
			return true
		}
	}
	return false
}

// workDirNextTo creates a private scratch directory beside the archive at
// localPath. It is next to the archive rather than in the system temp
// directory for two reasons: the finished archive is renamed from it onto
// the original, which is only atomic within one file system, and on a
// router the system temp directory is RAM, too small to hold a
// decompressed tarball.
func workDirNextTo(localPath string) (string, error) {
	dir, err := os.MkdirTemp(filepath.Dir(localPath), ".f4-multiarc-")
	if err != nil {
		return "", fmt.Errorf("multiarc: cannot create a work directory next to %s: %w", localPath, err)
	}
	return dir, nil
}

// replaceArchive moves the finished archive at built onto target, giving
// it perm first: the replacement keeps the original's permission bits
// rather than whatever the tool that wrote it chose.
func replaceArchive(built, target string, perm os.FileMode) error {
	if err := os.Chmod(built, perm); err != nil {
		return err
	}
	if err := os.Rename(built, target); err != nil {
		return fmt.Errorf("multiarc: cannot replace %s: %w", target, err)
	}
	return nil
}

// copyFile copies src to a new file dst with permission bits perm.
func copyFile(src, dst string, perm os.FileMode) (err error) {
	in, err := os.Open(filepath.Clean(src))
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }() // Read-only; nothing to lose on close.
	out, err := os.OpenFile(filepath.Clean(dst), os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}

// rewriteArchive runs edit on a private copy of the archive at localPath
// and, only when edit succeeds, moves the result it names back over the
// original. edit gets the work directory and the name of the copy inside
// it (copyName, relative to workDir) and returns the name, also relative,
// of the finished archive. The original is never touched before that last
// rename, so a failed or canceled edit leaves it exactly as it was.
func rewriteArchive(localPath, copyName string, edit func(workDir, copyName string) (string, error)) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	workDir, err := workDirNextTo(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(workDir) }() // Scratch only; the result was already moved out.
	if copyName != "" {
		if err := copyFile(localPath, filepath.Join(workDir, copyName), info.Mode().Perm()); err != nil {
			return fmt.Errorf("multiarc: cannot copy %s to edit it: %w", localPath, err)
		}
	}
	built, err := edit(workDir, copyName)
	if err != nil {
		return err
	}
	return replaceArchive(filepath.Join(workDir, built), localPath, info.Mode().Perm())
}
