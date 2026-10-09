package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// TestLiteBuildExcludesHeavyNetworkDependencies is the mechanical half of
// f4#1178 part 3: FISH+ over a subprocess ssh dialer came back into the lite
// build (plugins/netfox, gated file-by-file with //go:build lite/!lite --
// see internal/plughost/plugins_lite.go), and this is what keeps it from
// quietly dragging FTP, SFTP or Pageant support back in with it. Each of
// those links a library -tags lite exists to shed: github.com/jlaffaye/ftp,
// github.com/pkg/sftp, github.com/kbolino/pageant and
// golang.org/x/crypto/ssh (and its ssh/agent, ssh/knownhosts).
//
// This asks the toolchain rather than grepping source: a forbidden import
// reintroduced through a different file, or a new dependency of fishplus
// itself, is caught the same way an already-tagged one would be.
func TestLiteBuildExcludesHeavyNetworkDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/jlaffaye/ftp",
		"github.com/pkg/sftp",
		"github.com/kbolino/pageant",
		"golang.org/x/crypto/ssh",
	}

	deps := liteBuildDeps(t)

	var offenders []string
	for _, imported := range deps {
		for _, bad := range forbidden {
			if imported == bad || strings.HasPrefix(imported, bad+"/") {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a -tags lite build of ./cmd/f4 still depends on:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}

// TestLiteBuildStillIncludesFishPlus is the other side of the same check: an
// overzealous exclusion that dropped FISH+ itself back out of the lite build
// would pass the test above for the wrong reason. fishplus has no dependency
// beyond the standard library, so its presence here does not reintroduce any
// of the weight -tags lite sheds.
func TestLiteBuildStillIncludesFishPlus(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	deps := liteBuildDeps(t)
	const fishplus = "github.com/unxed/f4/plugins/netfox/fishplus"
	for _, imported := range deps {
		if imported == fishplus {
			return
		}
	}
	t.Fatalf("a -tags lite build of ./cmd/f4 no longer depends on %s", fishplus)
}

// TestLiteBuildExcludesArchiveLibraries keeps the archive library chain
// (docs/ARCHIVE_DEPENDENCIES.md) out of the lite build. Archives there go
// through plugins/multiarc, which runs whichever console archiver the host
// has, and internal/unpack reads f4's own release and plugin archives with
// the standard library (formats_lite.go). Before f4#1178 closed them, two
// leaks linked the whole chain anyway: internal/unpack's readers and
// internal/fileops' tar index bookkeeping, which needs nothing from
// github.com/unxed/tar but a sidecar file name.
//
// github.com/klauspost/compress's flate and zlib stay allowed: the PNG
// decoder (internal/media) uses them, and they are not an archive format.
func TestLiteBuildExcludesArchiveLibraries(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/unxed/f4/plugins/archive",
		"github.com/unxed/zipper",
		"github.com/unxed/zip",
		"github.com/unxed/tar",
		"github.com/unxed/sevenzip",
		"github.com/unxed/xz",
		"github.com/unxed/archives",
		"github.com/unxed/par2",
		"github.com/unxed/zlib4go",
		"github.com/unxed/zipcharset",
		"github.com/mholt/archives",
		"github.com/bodgit/sevenzip",
		"github.com/ulikunitz/xz",
		"github.com/nwaples/rardecode",
		"github.com/klauspost/pgzip",
		"github.com/klauspost/compress/zstd",
		"github.com/pierrec/lz4",
		"github.com/andybalholm/brotli",
		"github.com/stangelandcl/ppmd",
	}

	var offenders []string
	for _, imported := range liteBuildDeps(t) {
		for _, bad := range forbidden {
			if imported == bad || strings.HasPrefix(imported, bad+"/") {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a -tags lite build of ./cmd/f4 still depends on:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}

// TestLiteBuildStillIncludesMultiarc is the other side of the same check:
// archives must still open in a lite build, through the console-tool
// wrapper, not just stop linking.
func TestLiteBuildStillIncludesMultiarc(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const multiarc = "github.com/unxed/f4/plugins/multiarc"
	for _, imported := range liteBuildDeps(t) {
		if imported == multiarc {
			return
		}
	}
	t.Fatalf("a -tags lite build of ./cmd/f4 no longer depends on %s", multiarc)
}

// TestLiteBuildExcludesGPUBackends keeps the lite build's GUI to X11 and
// Wayland (Win32 on Windows). vtui's Ebitengine and gogpu backends bring
// Ebitengine, gogpu, wgpu, naga and gg, about 9 MB that stayed linked even
// while lite had no GUI at all, because those packages run init code.
func TestLiteBuildExcludesGPUBackends(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/hajimehoshi/ebiten",
		"github.com/gogpu/",
		"github.com/go-webgpu/webgpu",
	}
	var offenders []string
	for _, imported := range liteBuildDeps(t) {
		for _, bad := range forbidden {
			if strings.HasPrefix(imported, bad) {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a -tags %s build of ./cmd/f4 still depends on:\n\t%s", liteBuildTags, strings.Join(offenders, "\n\t"))
	}
}

// TestLiteBuildStillIncludesX11AndWayland is the other side of the same
// check: the lite build draws a window with X11 or Wayland. It asks about
// linux/amd64, the lite target that has both; vtui builds no Wayland
// backend on the host this may run on (macOS, Windows).
func TestLiteBuildStillIncludesX11AndWayland(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	deps := liteBuildDepsFor(t, "linux", "amd64")
	for _, want := range []string{"github.com/jezek/xgb", "github.com/neurlang/wayland/window"} {
		found := false
		for _, imported := range deps {
			if imported == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("a -tags %s build of ./cmd/f4 no longer depends on %s", liteBuildTags, want)
		}
	}
}

// TestLiteBuildStillIncludesWasmRuntime keeps wasm plugins in the lite
// build. f4#1178 once moved internal/plughost/transport_wazero.go behind
// //go:build !lite, but wazero weighs only about 1.5-2.7 MB there (4-5% of
// the stripped binary, least on the arm and mipsle targets lite is for), and
// a lite build without it cannot run the sandboxed plugins PlugRing is built
// around. A build tag that drops the transport again fails here.
func TestLiteBuildStillIncludesWasmRuntime(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const wazero = "github.com/tetratelabs/wazero"
	for _, imported := range liteBuildDeps(t) {
		if imported == wazero {
			return
		}
	}
	t.Fatalf("a -tags lite build of ./cmd/f4 no longer depends on %s", wazero)
}

// TestLiteBuildExcludesAudioPlayerDependencies is the mechanical half of
// f4#1178 part 5: the mp3/wav/flac/vorbis player was already kept out of a
// lite build by the `lite` tag on internal/media/audio_oto.go (part 1,
// PR #1469), but internal/media/audio_decode.go -- which does the actual
// decoding and is the file that imports github.com/ebitengine/oto/v3,
// github.com/hajimehoshi/go-mp3, github.com/jfreymuth/oggvorbis and
// github.com/mewkiz/flac -- carried no build tag of its own. All four
// libraries therefore kept linking into a lite build regardless of the
// player itself being unreachable there.
func TestLiteBuildExcludesAudioPlayerDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/ebitengine/oto/v3",
		"github.com/hajimehoshi/go-mp3",
		"github.com/jfreymuth/oggvorbis",
		"github.com/mewkiz/flac",
	}

	deps := liteBuildDeps(t)

	var offenders []string
	for _, imported := range deps {
		for _, bad := range forbidden {
			if imported == bad || strings.HasPrefix(imported, bad+"/") {
				offenders = append(offenders, imported)
			}
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("a -tags lite build of ./cmd/f4 still depends on:\n\t%s", strings.Join(offenders, "\n\t"))
	}
}

// TestRegularBuildStillIncludesAudioPlayerDependencies is the other side of
// the same check: an overzealous build tag that dropped the audio decoders
// out of a regular build too would pass the test above for the wrong
// reason. IsAudioFile/audioFormatFor (internal/media/audio_format.go) stay
// available in every build, but the decoders themselves are only reachable
// where audio_oto.go builds the real AudioEngine.
func TestRegularBuildStillIncludesAudioPlayerDependencies(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	deps := regularBuildDeps(t)
	want := []string{
		"github.com/ebitengine/oto/v3",
		"github.com/hajimehoshi/go-mp3",
		"github.com/jfreymuth/oggvorbis",
		"github.com/mewkiz/flac",
	}
	for _, lib := range want {
		found := false
		for _, imported := range deps {
			if imported == lib {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("a regular build of ./cmd/f4 no longer depends on %s", lib)
		}
	}
}

// TestRegularBuildStillIncludesWasmRuntime is the regular build's half of
// TestLiteBuildStillIncludesWasmRuntime.
func TestRegularBuildStillIncludesWasmRuntime(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const wazero = "github.com/tetratelabs/wazero"
	for _, imported := range regularBuildDeps(t) {
		if imported == wazero {
			return
		}
	}
	t.Fatalf("a regular build of ./cmd/f4 no longer depends on %s", wazero)
}

// TestLiteSheetPackageExcludesSQLiteDependency is the mechanical half of
// f4#1552: internal/sheet/store.go (the native ".f4s.sqlite" spreadsheet
// format) used to import github.com/ncruces/go-sqlite3/driver directly and
// unconditionally, in both builds -- a feature with nothing to do with
// either the sqlite plugin or tar/zipper archive indexing, and the reason
// f4#1178's own TestLiteBuildExcludesSQLiteDependency (below) had to Skip
// instead of asserting for real.
//
// f4#1552 closed that gap: store.go is now //go:build !lite, and
// store_lite.go (//go:build lite) implements the same Save/Load/IsSheetFile
// API on top of encoding/json instead. So this is now the bare "must not
// appear" assertion for internal/sheet specifically: if go-sqlite3 shows up
// in a lite build of that one package, it is a real regression, full stop.
func TestLiteSheetPackageExcludesSQLiteDependency(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const sqlite = "github.com/ncruces/go-sqlite3"

	sheetDeps := packageDepsWithTags(t, "lite", "./internal/sheet")
	var offenders []string
	for _, imported := range sheetDeps {
		if imported == sqlite || strings.HasPrefix(imported, sqlite+"/") {
			offenders = append(offenders, imported)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf(
			"a -tags lite build of ./internal/sheet still depends on:\n\t%s",
			strings.Join(offenders, "\n\t"),
		)
	}
}

// TestLiteBuildExcludesSQLiteDependency keeps github.com/ncruces/go-sqlite3,
// about 7 MB of a stripped lite binary, out of the lite build. Three things
// would link it there, and each has its own lite-side answer: the SQLite
// client (plugins/sqlite) runs the host's sqlite3 tool instead
// (backend_default_lite.go), internal/sheet saves as JSON (store_lite.go,
// f4#1552), and unxed/tar, whose archive index is SQLite-backed, is not
// linked at all (TestLiteBuildExcludesArchiveLibraries).
func TestLiteBuildExcludesSQLiteDependency(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const sqlite = "github.com/ncruces/go-sqlite3"

	deps := liteBuildDeps(t)
	var offenders []string
	for _, imported := range deps {
		if imported == sqlite || strings.HasPrefix(imported, sqlite+"/") {
			offenders = append(offenders, imported)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf(
			"a -tags %s build of ./cmd/f4 still depends on:\n\t%s",
			liteBuildTags, strings.Join(offenders, "\n\t"),
		)
	}
}

// TestRegularBuildStillIncludesSQLiteDependency is the other side of the
// same check: a regular (non-lite) build keeps both plugins/sqlite's
// in-process SQLite VFS mount and internal/sheet's native spreadsheet
// format, so github.com/ncruces/go-sqlite3 staying linked there is expected,
// not a regression. An overzealous change that dropped it out of a regular
// build too would pass TestLiteBuildExcludesSQLiteDependency for the wrong
// reason.
func TestRegularBuildStillIncludesSQLiteDependency(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const sqlite = "github.com/ncruces/go-sqlite3"
	for _, imported := range regularBuildDeps(t) {
		if imported == sqlite {
			return
		}
	}
	t.Fatalf("a regular build of ./cmd/f4 no longer depends on %s", sqlite)
}

// TestSQLiteClientIsInBothBuilds pins where the SQLite client lives: in
// both builds, in-process. The regular build links the engine for it (and
// for the spreadsheet and the archive index anyway); the lite build runs the
// host's sqlite3 tool, which TestLiteBuildExcludesSQLiteDependency holds it
// to. Shipping the client as a separate plugin binary instead would carry a
// second copy of the engine and gain nothing.
func TestSQLiteClientIsInBothBuilds(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	const client = "github.com/unxed/f4/plugins/sqlite"
	contains := func(deps []string) bool {
		for _, imported := range deps {
			if imported == client {
				return true
			}
		}
		return false
	}
	if !contains(regularBuildDeps(t)) {
		t.Errorf("a regular build of ./cmd/f4 no longer links %s", client)
	}
	if !contains(liteBuildDeps(t)) {
		t.Errorf("a -tags %s build of ./cmd/f4 no longer links %s", liteBuildTags, client)
	}
}

func regularBuildDeps(t *testing.T) []string {
	t.Helper()
	command := exec.Command("go", "list", "-deps", "./cmd/f4")
	command.Dir = testutil.ModuleRootDir(t)
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -deps ./cmd/f4: %v\n%s", err, stderr)
	}
	return strings.Fields(string(out))
}

// liteBuildTags is the tag set build-lite in .github/workflows/build.yml
// passes (minus its per-platform FFI tags): lite, plus the vtui tags that
// leave the Ebitengine and gogpu backends out. internal/gui/liteguard.go
// refuses to compile lite without them.
const liteBuildTags = "lite,vtui_noebiten,vtui_nogogpu"

// liteBuildDeps lists the packages a lite build of ./cmd/f4 links.
func liteBuildDeps(t *testing.T) []string {
	t.Helper()
	return liteBuildDepsFor(t, "", "")
}

// liteBuildDepsFor is liteBuildDeps for a given GOOS/GOARCH; empty means
// the host's.
func liteBuildDepsFor(t *testing.T, goos, goarch string) []string {
	t.Helper()
	command := exec.Command("go", "list", "-tags", liteBuildTags, "-deps", "./cmd/f4")
	command.Dir = testutil.ModuleRootDir(t)
	command.Env = os.Environ()
	if goos != "" {
		command.Env = append(command.Env, "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	}
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -tags %s -deps ./cmd/f4: %v\n%s", liteBuildTags, err, stderr)
	}
	return strings.Fields(string(out))
}

// packageDepsWithTags is liteBuildDeps/regularBuildDeps generalized to an
// arbitrary package and tag set, used by
// TestLiteSheetPackageExcludesSQLiteDependency to check a single package's
// own dependency graph (internal/sheet) rather than the whole ./cmd/f4
// build's.
func packageDepsWithTags(t *testing.T, tags, pkg string) []string {
	t.Helper()
	command := exec.Command("go", "list", "-tags", tags, "-deps", pkg)
	command.Dir = testutil.ModuleRootDir(t)
	out, err := command.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -tags %s -deps %s: %v\n%s", tags, pkg, err, stderr)
	}
	return strings.Fields(string(out))
}
