package multiarc

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// tarFlavor is which tar implementation is on PATH. Reading barely cares
// (every one of them lists and extracts the same way), but what each can
// change differs a great deal, so a write asks first.
type tarFlavor int

const (
	tarOther   tarFlavor = iota // unrecognized: OpenBSD's, Solaris', ...
	tarGNU                      // GNU tar: appends (-r) and deletes (--delete)
	tarBSD                      // bsdtar/libarchive (macOS, FreeBSD, Windows 10+'s tar.exe): appends only
	tarBusyBox                  // BusyBox's applet, common on routers: creates only
)

// tarInfo is what "tar --version" told about the tar on PATH.
type tarInfo struct {
	flavor tarFlavor
	// version is everything the probe printed, stdout and stderr together.
	// bsdtar lists the compression libraries built into it there.
	version string
}

// probeTar runs "tar --version". GNU tar and bsdtar print their name on
// stdout; BusyBox has no --version and names itself in the usage text it
// prints on stderr instead, so both streams are read and the exit status
// is not.
func probeTar(ctx context.Context) tarInfo {
	stdout, stderr, _ := runTool(ctx, "tar", "--version")
	text := string(stdout) + "\n" + string(stderr)
	info := tarInfo{version: text}
	switch {
	case strings.Contains(text, "GNU tar"):
		info.flavor = tarGNU
	case strings.Contains(text, "bsdtar"), strings.Contains(text, "libarchive"):
		info.flavor = tarBSD
	case strings.Contains(text, "BusyBox"):
		info.flavor = tarBusyBox
	}
	return info
}

func (i tarInfo) name() string {
	switch i.flavor {
	case tarGNU:
		return "GNU tar"
	case tarBSD:
		return "bsdtar"
	case tarBusyBox:
		return "BusyBox tar"
	}
	return "the tar on PATH"
}

// tarCompression is one compressor a tarball can be wrapped in.
type tarCompression struct {
	suffixes []string // lower-case archive name endings that select it
	ext      string   // what the stand-alone tool appends to a file it compresses
	tool     string   // the stand-alone compressor
	flag     string   // tar's own option for it
	lib      string   // how bsdtar --version names the library, when it has it built in
}

var tarCompressions = []tarCompression{
	{suffixes: []string{".tar.gz", ".tgz"}, ext: ".gz", tool: "gzip", flag: "-z", lib: "zlib/"},
	{suffixes: []string{".tar.bz2", ".tbz2", ".tbz", ".tb2"}, ext: ".bz2", tool: "bzip2", flag: "-j", lib: "bz2lib/"},
	{suffixes: []string{".tar.xz", ".txz"}, ext: ".xz", tool: "xz", flag: "-J", lib: "liblzma/"},
	{suffixes: []string{".tar.zst", ".tzst"}, ext: ".zst", tool: "zstd", flag: "--zstd", lib: "libzstd/"},
	{suffixes: []string{".tar.lz", ".tlz"}, ext: ".lz", tool: "lzip", flag: "--lzip", lib: "liblzma/"},
}

// tarCompressionFor maps an archive name to its compression: plain for a
// bare .tar, a tarCompressions entry otherwise. ok is false for a tarball
// multiarc reads but cannot write -- .taz, compress(1)'s .Z, whose
// compressor is long gone from most systems -- and for any other name.
func tarCompressionFor(name string) (comp tarCompression, plain, ok bool) {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar") {
		return tarCompression{}, true, true
	}
	for _, c := range tarCompressions {
		if hasAnySuffix(lower, c.suffixes...) {
			return c, false, true
		}
	}
	return tarCompression{}, false, false
}

// bsdCanCompress reports whether bsdtar can write comp: libarchive either
// has the library built in (it says so in --version) or falls back to the
// stand-alone tool when that is on PATH. Windows' tar.exe, for one, has
// zlib and bzip2 built in and nothing else.
func bsdCanCompress(info tarInfo, comp tarCompression) bool {
	return strings.Contains(info.version, comp.lib) || toolAvailable(comp.tool)
}

// tarMemberArgs prepares member names for a tar command line. bsdtar reads
// a name starting with "@" as another archive to copy entries from, "--" or
// not, so such a name goes in as "./@name"; the listing strips the "./"
// again. GNU and BusyBox tar give "@" no meaning.
func tarMemberArgs(info tarInfo, members []string) []string {
	if info.flavor != tarBSD {
		return members
	}
	out := make([]string, len(members))
	for i, m := range members {
		if strings.HasPrefix(m, "@") {
			m = "./" + m
		}
		out[i] = m
	}
	return out
}

// tarEditPlan is how one change to an existing tarball is carried out.
type tarEditPlan struct {
	info  tarInfo
	comp  tarCompression
	plain bool
}

// planTarEdit decides whether op can be done to the tarball at localPath
// with the tools on PATH, and how:
//
//   - GNU tar appends with -r and deletes with --delete, but only on an
//     uncompressed tar. A compressed one is decompressed into a private
//     copy with its stand-alone compressor, edited, and compressed again,
//     so the members themselves are carried over byte for byte -- nothing
//     is extracted to disk and re-archived, which would lose owners,
//     device nodes and anything else a non-root f4 cannot recreate.
//   - bsdtar appends with -r to an uncompressed tar, and to a compressed
//     one by writing a new archive that copies every existing entry as is
//     ("@archive") and adds the new members after them. It has no
//     --delete, and the only way to drop members with it is --exclude,
//     whose patterns are unanchored ("a.txt" also drops "dir/a.txt"), so
//     deleting is refused rather than risked. Replacing is refused for the
//     same reason: without a delete, the old member would stay behind.
//   - BusyBox tar, and any tar not recognized, can do neither.
func planTarEdit(ctx context.Context, localPath string, op writeOp) (tarEditPlan, error) {
	name := filepath.Base(localPath)
	comp, plain, ok := tarCompressionFor(name)
	if !ok {
		return tarEditPlan{}, fmt.Errorf("multiarc: %s cannot be changed: only .tar and gzip, bzip2, xz, zstd or lzip compressed tarballs can", name)
	}
	info := probeTar(ctx)
	switch info.flavor {
	case tarGNU:
		if !plain && !toolAvailable(comp.tool) {
			return tarEditPlan{}, fmt.Errorf("multiarc: changing %s needs %s on PATH", name, comp.tool)
		}
	case tarBSD:
		switch op {
		case writeRemove:
			return tarEditPlan{}, fmt.Errorf("multiarc: bsdtar cannot delete members from %s; GNU tar is needed for that", name)
		case writeReplace:
			return tarEditPlan{}, fmt.Errorf("multiarc: bsdtar cannot replace a member of %s, only add new ones; GNU tar is needed for that", name)
		}
		if !plain && !bsdCanCompress(info, comp) {
			return tarEditPlan{}, fmt.Errorf("multiarc: changing %s needs %s, which this bsdtar lacks", name, comp.tool)
		}
	default:
		return tarEditPlan{}, fmt.Errorf("multiarc: %s cannot change an existing archive; GNU tar or bsdtar is needed for that", info.name())
	}
	return tarEditPlan{info: info, comp: comp, plain: plain}, nil
}

func (tarBackend) checkWrite(ctx context.Context, localPath string, op writeOp, _ string) error {
	_, err := planTarEdit(ctx, localPath, op)
	return err
}

func (tarBackend) add(ctx context.Context, localPath, stageDir string, members, replaced []string) error {
	op := writeAdd
	if len(replaced) > 0 {
		op = writeReplace
	}
	plan, err := planTarEdit(ctx, localPath, op)
	if err != nil {
		return err
	}
	names := tarMemberArgs(plan.info, members)
	if plan.info.flavor == tarBSD && !plan.plain {
		// One command, never chunked: each run writes a whole new archive
		// from the original, so a second chunk would drop the first. Option
		// order matters to bsdtar: it stops reading options at the first
		// name, and "@archive" is a name, read after -C has already moved it
		// into stageDir -- hence the absolute localPath.
		out := "work.tar" + plan.comp.ext
		return rewriteArchive(localPath, "", func(workDir, _ string) (string, error) {
			args := append([]string{"-c", plan.comp.flag, "-f", out, "-C", stageDir, "--", "@" + localPath}, names...)
			return out, runToolChecked(ctx, workDir, "tar", args...)
		})
	}
	return editPlainTar(ctx, localPath, plan, func(workDir, tarName string) error {
		if len(replaced) > 0 {
			if err := deleteFromTar(ctx, workDir, tarName, replaced); err != nil {
				return err
			}
		}
		// Runs in stageDir rather than naming it with "-C stageDir" (see
		// backend_tar.go's extractAll for why: GNU tar fails to open a
		// Windows drive-letter "-C" directory), so the archive itself,
		// relative to workDir otherwise, is named by its absolute path
		// instead -- bsdtar takes that plainly, GNU tar needs
		// --force-local for it, the same as everywhere else one is built.
		head := []string{"-r"}
		if plan.info.flavor == tarGNU {
			head = append(head, "--force-local")
		}
		head = append(head, "-f", filepath.Join(workDir, tarName))
		return runChunked(ctx, stageDir, "tar", head, names)
	})
}

func (tarBackend) remove(ctx context.Context, localPath string, raws []string) error {
	plan, err := planTarEdit(ctx, localPath, writeRemove)
	if err != nil {
		return err
	}
	return editPlainTar(ctx, localPath, plan, func(workDir, tarName string) error {
		return deleteFromTar(ctx, workDir, tarName, raws)
	})
}

// deleteFromTar runs GNU tar --delete on tarName in workDir. --no-wildcards
// is its default for member names already, stated anyway so that
// TAR_OPTIONS cannot change it; the names are the covering ones, because
// GNU tar fails on a name whose members an earlier one already deleted.
func deleteFromTar(ctx context.Context, workDir, tarName string, raws []string) error {
	return runChunked(ctx, workDir, "tar", []string{"--delete", "--no-wildcards", "-f", tarName}, coveringNames(raws))
}

// editPlainTar hands edit an uncompressed copy of the tarball at localPath,
// named relative to the work directory it runs in, and puts the result
// back, compressed again the way it was.
func editPlainTar(ctx context.Context, localPath string, plan tarEditPlan, edit func(workDir, tarName string) error) error {
	const tarName = "work.tar"
	copyName := tarName + plan.comp.ext // plain: comp is zero, ext is ""
	return rewriteArchive(localPath, copyName, func(workDir, copyName string) (string, error) {
		if !plan.plain {
			// "gzip -d -f work.tar.gz" leaves work.tar, and so do bzip2, xz,
			// zstd and lzip for their own extensions -- BusyBox's gzip
			// included, which is why no option beyond -d and -f is used.
			if err := runToolChecked(ctx, workDir, plan.comp.tool, "-d", "-f", copyName); err != nil {
				return "", err
			}
		}
		if err := edit(workDir, tarName); err != nil {
			return "", err
		}
		if plan.plain {
			return tarName, nil
		}
		if err := runToolChecked(ctx, workDir, plan.comp.tool, "-f", tarName); err != nil {
			return "", err
		}
		return copyName, nil
	})
}
