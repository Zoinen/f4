package multiarc

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

// errNoGzip is what a .gz write answers when only gunzip is on PATH: it
// decompresses, it does not compress.
var errNoGzip = errors.New("multiarc: changing a .gz file needs gzip on PATH (gunzip only decompresses)")

// checkWrite: a lone .gz holds exactly one file, so the one change that
// makes sense is new content for that file -- saving it from the editor, or
// copying a file of the same name over it. Anything else is refused with
// the reason.
func (b gzipBackend) checkWrite(_ context.Context, localPath string, op writeOp, member string) error {
	name := filepath.Base(localPath)
	inner := b.innerName(localPath)
	switch op {
	case writeRemove:
		return fmt.Errorf("multiarc: %s holds exactly one file; delete %s itself instead", name, name)
	case writeMkDir:
		return fmt.Errorf("multiarc: %s holds exactly one file and cannot hold a directory", name)
	}
	if member != inner {
		return fmt.Errorf("multiarc: %s holds exactly one file, %s, and cannot take another", name, inner)
	}
	if !toolAvailable("gzip") {
		return errNoGzip
	}
	return nil
}

// add compresses the staged file into a new .gz beside the old one and
// renames it over. gzip names its output after its input, so the staged
// file is copied into the work directory under the member's own name first.
func (b gzipBackend) add(ctx context.Context, localPath, stageDir string, members, _ []string) error {
	if len(members) != 1 {
		return fmt.Errorf("multiarc: %s holds exactly one file", filepath.Base(localPath))
	}
	inner := members[0]
	if err := b.checkWrite(ctx, localPath, writeReplace, inner); err != nil {
		return err
	}
	return rewriteArchive(localPath, "", func(workDir, _ string) (string, error) {
		if err := copyFile(filepath.Join(stageDir, inner), filepath.Join(workDir, inner), 0o600); err != nil {
			return "", err
		}
		if err := runToolChecked(ctx, workDir, "gzip", "-f", "--", inner); err != nil {
			return "", err
		}
		return inner + ".gz", nil
	})
}

func (b gzipBackend) remove(ctx context.Context, localPath string, _ []string) error {
	return b.checkWrite(ctx, localPath, writeRemove, "")
}
