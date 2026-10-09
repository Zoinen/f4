# Archive libraries: the dependency chain

f4 reads and writes archives through a chain of Go libraries. Most of them
live in their own repositories under github.com/unxed. This document says
which library does what, how they depend on each other, and the rules for
updating them. Read it before you change a line in any `go.mod` that names
one of these libraries.

`scripts/check_archive_deps.sh` checks the rules below. Run it after every
change to the chain.

## Which library does what

| Module | What it does | Repository |
| --- | --- | --- |
| `github.com/unxed/zipper` | One API over all formats (its `archive` package, used by f4), plus the `zipper` command-line tool | our own |
| `github.com/unxed/zip` | ZIP | our own |
| `github.com/unxed/tar` | TAR, compressed or not (.tar.gz, .tar.xz and others) | our own |
| `github.com/unxed/xz` | XZ, LZMA and LZMA2; used by zip, tar, sevenzip and archives | fork of `ulikunitz/xz` |
| `github.com/unxed/sevenzip` | 7z | fork of `bodgit/sevenzip` |
| `github.com/unxed/archives` | RAR and every other format | fork of `mholt/archives` |
| `github.com/nwaples/rardecode/v2` | RAR decoding, used by archives, zipper and f4 | not forked, see rule 8 |
| `github.com/unxed/par2` | Recovery records (PAR2), used by zip and tar | our own |
| `github.com/unxed/zipcharset` | Old non-Unicode file names in ZIP | our own |
| `github.com/unxed/localecp` | Finds the system code page | our own |
| `github.com/unxed/zlib4go` | zlib, bit-for-bit like the C library | our own |

Inside f4 the archive code is in `plugins/archive` (panels, copying, testing),
`internal/unpack` (extracting), `internal/fileops` and `internal/editor`.
Self-extracting archives (SFX, `.exe`) are handled by f4 itself in
`plugins/archive/sfx.go`; zipper does not know about them.

## How they depend on each other

An arrow means "imports". A `go.mod` also lists what its imports pull in,
marked `// indirect`, and the rules below apply to those lines too.

```
f4         ──► zipper, zip, tar, sevenzip, archives, rardecode
zipper     ──► zip, tar, sevenzip, archives, rardecode, par2
zip        ──► xz, par2, zipcharset, zlib4go
tar        ──► xz, par2
archives   ──► sevenzip, xz, rardecode
sevenzip   ──► xz
zipcharset ──► localecp
```

So the order from the bottom up is:

1. `xz`, `par2`, `zlib4go`, `localecp`, `rardecode`
2. `zipcharset`, `sevenzip`
3. `zip`, `tar`, `archives`
4. `zipper`
5. `f4`

## How Go chooses a version

Every `go.mod` names the lowest version it needs of each library. When a
program is built, Go takes, for each library, the highest version that any
`go.mod` in the build asks for.

This has three consequences:

- f4 gets a fix as soon as f4's own `go.mod` asks for the fixed version, even
  if a library in the middle still asks for an older one.
- That library, built or tested on its own, still gets the older version. Its
  tests then check code that f4 does not use, and so does the `zipper` tool.
  This is why every library must ask for the same versions as f4 (rule 3).
- A `replace` line works only in the `go.mod` of the program being built. A
  `replace` in zipper's `go.mod` is ignored when f4 is built.

## Rules

1. **Refer to a library by a tag**, like `v0.1.45`. Never by a commit or a
   branch. A version that points at a commit is called a pseudo-version and
   looks like `v0.0.0-20260925074956-1fd8e92986d5` or
   `v0.1.140-0.20260924053511-cd3c7523a2c8`.
2. **No `replace` for these libraries in a commit.** zipper's `go.mod` has
   `replace` lines for working with local copies; they must stay commented
   out when you commit.
3. **Every library asks for the versions f4 is built with.** If f4 is built
   with xz v0.1.45, then zip, tar, sevenzip, archives and zipper also ask for
   xz v0.1.45.
4. **A change goes all the way up.** When you change a library, update every
   library above it, in the order above, up to f4. Do not stop half way. The
   steps are in "How to bring a change to f4" below.
5. **A new tag when the code or `go.mod` changes.** A change only in tests, CI
   or documentation needs no tag: nobody who uses the library gets it.
6. **Never move, delete or reuse a tag after you pushed it.** The Go module
   proxy (proxy.golang.org) remembers every version it has seen, forever,
   even after the tag is deleted on GitHub. Example: `unxed/sevenzip` had tags
   v0.1.0 and v0.1.1 in June 2026 that were deleted later. The proxy still
   gives the June code for them, and v0.1.0 there even has the wrong module
   path. So before you choose a tag, look at both lists and take a number
   higher than everything in them:

   ```
   git tag --sort=-v:refname
   curl -s https://proxy.golang.org/github.com/unxed/<name>/@v/list
   ```

   Do not ask the proxy for a version before its tag is pushed (for example
   with `go get ...@v0.1.5` or by opening `.../@v/v0.1.5.info`). The proxy then
   answers "not found" for that version for up to 30 minutes, even after you
   push the tag.
7. **Forks: our code is on `write`, `main` is a copy of upstream.** This is
   about `unxed/sevenzip` and `unxed/archives`. See "The forks" below.
8. **rardecode is not forked and not changed here.** For licensing reasons,
   RAR decoding is not changed in this project. Report RAR problems to
   github.com/nwaples/rardecode as issues.

## How to bring a change to f4

Example: you fixed something in `unxed/tar`. Run each command on its own and
check its output before the next one.

1. In your clone of `unxed/tar`, commit and push the fix to `main`.
2. Choose the next tag as rule 6 says, for example `v0.1.134`. Then:

   ```
   git tag v0.1.134
   git push origin v0.1.134
   ```

3. Find everything that asks for `tar` in the chain above: `zipper` and
   `f4`. In your clone of `unxed/zipper`, on `main`:

   ```
   git pull --ff-only
   go get github.com/unxed/tar@v0.1.134
   go mod tidy
   git commit -am "deps: tar v0.1.134"
   git push origin main
   ```

   Then tag zipper the same way as in step 2.
4. In your clone of `unxed/f4`, on `main`, ask for both new versions:

   ```
   git pull --ff-only
   go get github.com/unxed/tar@v0.1.134 github.com/unxed/zipper@<new zipper tag>
   ./scripts/check_archive_deps.sh
   git commit -am "deps: tar v0.1.134 through zipper <new zipper tag>"
   git push origin main
   ```

   Commit only when the script prints `OK`.

For `xz` the path is longer: `sevenzip` first, then `zip`, `tar` and
`archives`, then `zipper`, then f4. Tag each one before you update the next.

## The forks: sevenzip and archives

`unxed/sevenzip` and `unxed/archives` are forks. In both:

- `write` is the default branch. It holds our code, its `go.mod` says
  `module github.com/unxed/...`, and tags go only on it.
- `main` is an exact copy of upstream `main`. Never commit to it. Its
  `go.mod` names the upstream module (`github.com/bodgit/sevenzip`,
  `github.com/mholt/archives`), so Go cannot use it under our name.
- A change for upstream is made on a new branch from `main`, and sent to
  upstream as a pull request from that branch.

To bring upstream changes in (sevenzip shown, for archives use
`https://github.com/mholt/archives.git`):

Once per clone:

```
git remote add upstream https://github.com/bodgit/sevenzip.git
git config remote.upstream.tagOpt --no-tags
```

Every time:

```
git fetch upstream
git push origin upstream/main:main
git switch write
git merge upstream/main
```

The `--no-tags` setting keeps upstream tags out of the fork: they would mix
with our tags, and they point at code whose `go.mod` has the upstream module
path. After the merge, run the tests, push `write`, and bring the change to f4
as described above.

## In f4's go.mod but not in f4

`github.com/mholt/archives` and `github.com/bodgit/sevenzip` are used only by
`plugins/archive/archive_test.go`. `github.com/ulikunitz/xz` comes in only
through `github.com/sorairolake/lzip-go`, for `.lz` files. None of them is
part of the chain. To see why a module is in `go.mod`:

```
go mod why -m github.com/mholt/archives
```

## Checking a bug outside f4

The `zipper` tool is built from the same libraries. If a problem shows up in
f4 but not in `zipper l` or `zipper x` on the same archive, the problem is in
f4. This holds only while rule 3 holds, and not for self-extracting archives,
which zipper does not open.
