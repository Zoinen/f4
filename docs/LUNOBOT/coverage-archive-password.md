# Coverage: `plugins/archive/password.go`

## Scope

This change adds deterministic tests for the archive password layer:

- lazy 7z read-error classification and ZipCrypto payload errors;
- retrying empty password answers and stopping on context or prompt errors;
- the no-UI password-prompt guard;
- password installation for value and pointer RAR/7z formats, including the
  empty-password and unsupported-format paths.

## Baseline

The file was selected from the fresh Codecov report after merge commit
`9b23c041e953ded7218301b2eaca9048b4265768`: `plugins/archive/password.go`
had 47.56% line coverage for 164 lines, with 75 missed lines. Total
repository coverage was 60.98%.

## Verification

No local Go build or test is run, per the current LUNOBOT passport. Static
checks are `gofmt` and `git diff --check`; GitHub Actions is the authoritative
verification environment.

The first full CI attempt, run `34338222185`, had one unrelated failure:
`Test (linux/arm64)` reached the 15-minute global test timeout in shuffled
`cmd/f4` tests, while `plugins/archive` passed. Rerunning only the failed job
completed successfully in 1m36s; all required build, test, race, lint, vet,
quality, and Codecov checks are green.

PR [#1048](https://github.com/unxed/f4/pull/1048) was merged on 2026-09-09
with merge commit `f73592bcd807d34f5fe2a14e9338ec5bff21cc05`.
