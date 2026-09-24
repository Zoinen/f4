# Issue #1179 (part 1, 7z) solution review — SFX detection, SFX testing, dialog hotkeys, split volumes

Scope: points 1, 2, 2a and 3 of the issue.

## Point 1: the 7-Zip installer fails with `sevenzip: checksum error`

Measured on `7z2603-x64.exe` (the file linked from the issue):

| offset | what is there | start header CRC (bytes 8..11 vs CRC32 of 12..31) |
| --- | --- | --- |
| 36192 | 7z signature inside the installer stub, followed by stub data | stored `40000000`, computed `e47a9c70` — mismatch |
| 45568 | the real archive (7-Zip 26.03 reports `Offset = 45568`) | `4ebfa010` both — match |

`findEmbeddedArchive` returned the first signature it met, 36192. Cutting the
file at each offset and opening the result with zipper reproduces the report
exactly at 36192 (`checksum error`) and lists all members at 45568, so the wrong
offset is the cause.

7-Zip does not trust the signature alone either: `FindAndReadSignature` in
`CPP/7zip/Archive/7z/7zIn.cpp` accepts a match only when `TestStartCrc` holds.
The probe now applies the same test to 7z candidates and keeps searching past a
rejected one. ZIP and RAR candidates are unchanged.

## Point 2: an SFX opens in the panel but "Test" says `no formats matched`

Panel entry (`NewArchiveVFSContext`) copies the embedded archive to a private
backing file and reads that. Testing (`actionTestArchive`) and Shift-F2 on the
file (`actionExtractArchive`) passed the original `.exe` path straight to
`archives.Identify` / zipper — from inside the archive too, because
`LocalArchivePath` returns the `.exe`. Neither knows about SFX stubs, hence
`no formats matched`. Shift-F2 on an SFX file failed the same way
(`failed to identify archive format`), which the issue did not mention.

Both now resolve the path through `localArchiveBacking`, which shares
`materializeLocalSFX` with panel entry, once per operation (not per password
attempt), and remove the copy when done. The progress and error texts still
name the `.exe`.

Note for checking with zipper: SFX support lives in f4 (`plugins/archive/sfx.go`),
zipper has none, so `zipper l some-sfx.exe` answering `no formats matched` says
nothing about f4.

## Point 2a: "Copy list" and "Close" share the hotkey C

The labels were `&Copy list` and `&Close`. Copy is now `Copy &list`. The
command-history details dialog had the same clash (`&Close` / `&ChDir`);
ChDir is now `Ch&Dir`.

## Verified on real files

After the change, on Linux: `7z2603-x64.exe` enters, tests and extracts (108
entries); 7z SFX archives built with the installer's own `7z.sfx` and
`7zCon.sfx` enter, test and extract; a plain `.7z` behaves as before.

## Point 3: `test.7z.001` fails with `error reading header id: EOF`

7-Zip volumes (`-v`) are a plain byte split: the volumes of the reproduction,
concatenated, are byte-identical to the unsplit archive. Handed only the first
volume, `archives.SevenZip.Extract` fails exactly as reported; handed the
joined volumes, the same call lists every member. f4 and zipper both passed a
stream of the first file only.

zipper now provides `archive.OpenInput`, which reads `.001`, `.002`, ... up to
the first missing number as one stream (the rule `sevenzip.OpenReader`
documents for a `.001` name); zipper's own FS and extractor use it, so panel
entry and Shift-F2 work. f4 opened the archive file itself in three more
places, all now through `OpenInput`:

| operation | before | after |
| --- | --- | --- |
| F5 out of the archive (`openBulkExtractor`) | `error reading header id: EOF` | copies |
| Shift-F3 (`openArchiveTestStream`) | `error reading header id: EOF` | no errors |
| check after Shift-F2 (`validateExtracted7z`) | skipped: gated on a `.7z` extension | runs for `name.7z.001` |

Measured on `vol.7z.001` built with `7zz a -v1m`; entering, F3 on a member and
scanning worked already once zipper joined the volumes.

7-Zip's `-sfx` together with `-v` writes a separate stub `name.exe` plus plain
volumes `name.exe.001`, ... . Those enter, copy, test and extract like any
split archive; the post-extraction check stays gated on a `.7z` name, as for
any other 7z archive not named `.7z`.
