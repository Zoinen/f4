# Issue #309 solution review

The screenshots show the bundled `hello-plugring` entry failing in the
published Windows build because its asset URL is `file://plugring/...`. That
relative local path only exists when f4 is started from a source checkout; a
GitHub release binary starts with an unrelated working directory and does not
contain the repository's `plugring` directory.

I compared three approaches:

1. Resolve the relative `file://` path against the process working directory.
   This is the current behavior and still fails for installed binaries.
2. Copy the test plugin beside every release binary or teach the installer to
   locate an executable-relative copy. This makes the catalog depend on a
   release-packaging detail and does not work for a catalog downloaded from
   GitHub.
3. Make the catalog asset a stable absolute HTTPS URL and add a regression
   test that rejects non-HTTPS bundled assets. This is the chosen solution:
   the same catalog entry works from a checkout and from a published binary,
   without changing the installer's handling of normal remote assets.
