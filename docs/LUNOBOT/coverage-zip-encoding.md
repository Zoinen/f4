# Coverage: `plugins/archive/zip_encoding.go`

## Scope

This change covers ZIP filename decoding for:

- names already marked as UTF-8;
- CP866 names from DOS creators 0, 6, and old OS=11 archives;
- Windows-1251 names from modern OS=11 archives;
- CP437 fallback for an unknown creator.

## Baseline

The file was selected from the live Codecov snapshot after the background-jobs
coverage task: `plugins/archive/zip_encoding.go` had 0% line coverage for 17
lines of real encoding-selection and decoding logic. The total repository
coverage in that snapshot was 60.87%.

## Verification

No local Go build or test is run, per the current LUNOBOT passport. Static
checks are `gofmt` and `git diff --check`; GitHub Actions is the authoritative
verification environment.

## Verification

The first CI attempt `34336394992` was cancelled after its Windows-1251 test
fixture used the CP866 byte for `т`. The corrected head
`af2339823b0d1aa61bfa3a75f0984c2787195ded` passed all build, test, race, lint,
vet, Quality, and Codecov checks in run [34336700811](https://github.com/unxed/f4/actions/runs/34336700811).

The change was merged in PR [#1046](https://github.com/unxed/f4/pull/1046),
merge commit `0501d966a772b52c28ee92252bd6f6a2197df36e`.
