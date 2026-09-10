# Coverage: `vfs/hostfs`

## Status

The Codecov report for the merge commit
`4cd62a34e083c0c7fcf47eedbee4a1a69b8a9831` reported 59.63% project coverage
and 21.18% for `vfs/hostfs`, the least-covered free package with real logic.
This slice adds filesystem-operation coverage for the POSIX and native
Windows forwarding layers, plus Windows-only checks for error normalization,
metadata conversion, open-flag translation, and directory-entry helpers.

Local builds and tests were not run. GitHub Actions is the authoritative
verification for this change.

The initial PR commit `460adffdd445c7109f0fe0dec7bb3cfe0198ad71` passed the
full GitHub Actions run [34293187360](https://github.com/unxed/f4/actions/runs/34293187360),
including the Windows A-D tests for the Windows-only helper paths.
