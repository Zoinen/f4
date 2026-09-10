# Issue #278 solution review

## Reproduction

The report compares the same directory in far2l and f4. A directory containing
`_test_folder`, `9test`, `archive`, and `unxed` is a minimal reproduction when
the panel is sorted by name: f4's previous code used `strings.ToLower(name)` and
therefore placed `_test_folder` after the alphabetic names. The supplied
screenshots show the same family of leading-symbol cases and the case where the
behavior was not visible because the active sort state differed.

## Three candidate solutions

1. Prefix or replace underscores with a hand-written sentinel before comparing.
   This fixes the named example but is incorrect for other punctuation,
   non-ASCII symbols, and extension sorting, and would diverge from far2l in
   more cases.

2. Call a platform-specific comparator such as Windows `CompareString` and
   provide unrelated implementations on Unix. This could match one host's
   locale but would make panel order vary by operating system and would not be
   available for every VFS or cross-build.

3. Use the portable Unicode string collator from `golang.org/x/text`, with
   case ignored and a forced identity tie-break. Apply it consistently to name
   sorting, extension sorting, and equal primary keys. This follows far2l's
   string-collation model without a one-character special case and keeps order
   deterministic across platforms.

The third solution is selected.

## Three-pass review

### Pass 1: correctness

`sortEntries` now compares names with `collate.New(language.Und,
collate.IgnoreCase, collate.Force)`. Leading punctuation sorts before digits and
letters, so `_test_folder` precedes `9test`, `archive`, and `unxed`. Extension
sorting uses the same comparator, and equal size/time/extension keys fall back
to names. Parent-directory and directory-first rules remain outside the name
comparator, so `..` and folders keep their existing placement.

### Pass 2: concurrency and stability

The collator is created per sort operation because its comparison method reuses
internal iterator state and is not safe for concurrent calls. The comparator
returns false for equal names in both directions and applies reverse ordering
by negating a three-way comparison, removing the previous `!res` equal-key
violation in reverse mode.

### Pass 3: scope and regression safety

Production behavior changes only the ordering of sortable panel entries. New
tests cover the reported leading-underscore order and equal-key reverse sorting;
the existing name, size, time, and sort-mode tests remain green. No VFS,
rendering, or file-operation behavior is changed.
